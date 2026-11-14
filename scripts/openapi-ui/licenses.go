package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var permittedLicenses = []string{"MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "ISC", "0BSD", "CC0-1.0", "Unlicense", "BlueOak-1.0.0", "CC-BY-4.0", "Python-2.0"}
var tokenPattern = regexp.MustCompile(`[()]|[^\s()]+`)

type expressionParser struct {
	tokens   []string
	position int
}

func (p *expressionParser) accept(token string) bool {
	if p.position < len(p.tokens) && p.tokens[p.position] == token {
		p.position++
		return true
	}
	return false
}

func (p *expressionParser) atom() (bool, bool) {
	if p.accept("(") {
		allowed, valid := p.disjunction()
		return allowed, valid && p.accept(")")
	}
	if p.position >= len(p.tokens) {
		return false, false
	}
	token := p.tokens[p.position]
	p.position++
	return slices.Contains(permittedLicenses, token), token != ")" && token != "AND" && token != "OR"
}

func (p *expressionParser) conjunction() (bool, bool) {
	allowed, valid := p.atom()
	for valid && p.accept("AND") {
		other, ok := p.atom()
		allowed, valid = allowed && other, ok
	}
	return allowed, valid
}

func (p *expressionParser) disjunction() (bool, bool) {
	allowed, valid := p.conjunction()
	for valid && p.accept("OR") {
		other, ok := p.conjunction()
		allowed, valid = allowed || other, ok
	}
	return allowed, valid
}

func permitted(expression string) bool {
	p := expressionParser{tokens: tokenPattern.FindAllString(expression, -1)}
	allowed, valid := p.disjunction()
	return valid && allowed && p.position == len(p.tokens)
}

var reviewed = map[string]struct{ license, hash string }{
	"decko@1.2.0":        {"MIT", "d20ef4c192faa3c560bc50bdd780f686804e355957d4146ddf8cbb58eb2e3dba"},
	"stickyfill@1.1.1":   {"MIT", "82183d79743926de3202987112039cda376a447a07422d45de1392cd85e40d07"},
	"url-template@2.0.8": {"BSD-3-Clause", "70723b90e3f26aa2808616e846cd8cd349fe33864fd328a91a5bb9d33e58e9d9"},
}

type notice struct {
	text string
	info dependency
}

func (u updater) dependencyNotice(location string, entry lockEntry) (notice, error) {
	const marker = "node_modules/"
	name := location[strings.LastIndex(location, marker)+len(marker):]
	identity := name + "@" + entry.Version
	contents, err := u.archive(entry)
	if err != nil {
		return notice{}, err
	}
	var metadata struct {
		License  any
		Licenses []struct{ Type string }
	}
	if err := json.Unmarshal(contents["package.json"], &metadata); err != nil {
		return notice{}, err
	}
	expression := entry.License
	if expression == "" {
		switch license := metadata.License.(type) {
		case string:
			expression = license
		case map[string]any:
			expression, _ = license["type"].(string)
		}
		if expression == "" && metadata.License == nil {
			var options []string
			for _, license := range metadata.Licenses {
				options = append(options, license.Type)
			}
			expression = strings.Join(options, " OR ")
		}
	}
	if approved, ok := reviewed[identity]; ok {
		if digest(contents["LICENSE"]) != approved.hash {
			return notice{}, fmt.Errorf("Reviewed license text changed: %s", identity)
		}
		expression = approved.license
	}
	if !permitted(expression) {
		return notice{}, fmt.Errorf("License review required: %s (%s)", identity, expression)
	}
	notices := make(map[string][]byte)
	for name, content := range contents {
		if noticePattern.MatchString(name) {
			notices[name] = content
		}
	}
	if len(notices) == 0 {
		overrides := make(map[string]struct{ File, License, SHA256, Source string })
		if err := u.readJSON(toolsPath+"/license-overrides.json", &overrides); err != nil {
			return notice{}, err
		}
		if override, ok := overrides[identity]; ok {
			data, err := os.ReadFile(filepath.Join(u.root, toolsPath, override.File))
			if err != nil {
				return notice{}, err
			}
			if override.License != expression || digest(data) != override.SHA256 {
				return notice{}, fmt.Errorf("Supplementary license text changed: %s", identity)
			}
			notices[override.Source] = data
		} else {
			if !slices.Contains([]string{"MIT", "Apache-2.0", "CC0-1.0"}, expression) {
				return notice{}, fmt.Errorf("Redistribution notice review required: %s", identity)
			}
			data, err := os.ReadFile(filepath.Join(u.root, toolsPath, "licenses", expression+".txt"))
			if os.IsNotExist(err) {
				return notice{}, fmt.Errorf("Redistribution notice review required: %s", identity)
			}
			if err != nil {
				return notice{}, err
			}
			notices["original package.json"] = contents["package.json"]
			for key, value := range contents {
				if strings.HasPrefix(strings.ToLower(key), "readme") {
					notices[key] = value
				}
			}
			notices[expression+" license terms"] = data
		}
	}
	var text strings.Builder
	fmt.Fprintf(&text, "\n%s\n%s\nLicense: %s\nSource: %s\n", strings.Repeat("=", 72), identity, expression, entry.Resolved)
	keys := make([]string, 0, len(notices))
	for key := range notices {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if !utf8.Valid(notices[key]) {
			return notice{}, fmt.Errorf("invalid UTF-8 notice: %s/%s", identity, key)
		}
		fmt.Fprintf(&text, "\n--- %s ---\n%s\n", key, notices[key])
	}
	return notice{text.String(), dependency{name, entry.Version, expression, entry.Integrity}}, nil
}
