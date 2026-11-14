package sourceproof

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"
)

const core = "github.com/NARUBROWN/spine"

var managed = map[string]bool{
	core: true, core + "-echo-platform": true, core + "-nethttp-platform": true, core + "-gorilla-platform": true,
	core + "-kafka-platform": true, core + "-rabbitmq-platform": true, core + "-nats-platform": true, core + "-validator-playground": true,
}
var tag = regexp.MustCompile(`^(v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))(?:-(preview|rc)\.[1-9][0-9]*)?$`)

func base(version string) (string, error) {
	match := tag.FindStringSubmatch(version)
	if match == nil {
		return "", fmt.Errorf("invalid release tag %q", version)
	}
	return match[1], nil
}

// canonicalJSON preserves the ASCII escaping used by existing manifests.
func canonicalJSON(value any) ([]byte, error) {
	var raw bytes.Buffer
	enc := json.NewEncoder(&raw)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	var result strings.Builder
	for _, r := range strings.TrimSuffix(raw.String(), "\n") {
		if r < 127 {
			result.WriteRune(r)
			continue
		}
		if r <= 0xffff {
			fmt.Fprintf(&result, `\u%04x`, r)
			continue
		}
		hi, lo := utf16.EncodeRune(r)
		fmt.Fprintf(&result, `\u%04x\u%04x`, hi, lo)
	}
	return []byte(result.String()), nil
}

func normalizePolicy(content []byte) ([]byte, error) {
	var p map[string]any
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&p); err != nil {
		return nil, err
	}
	for _, key := range []string{"ci_sources", "ci_core", "post_release_examples"} {
		delete(p, key)
	}
	channels, ok := p["channels"].(map[string]any)
	if !ok {
		return nil, errors.New("release policy channels missing")
	}
	for _, raw := range channels {
		selected, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("invalid release channel")
		}
		version, _ := selected["core_version"].(string)
		normalized, err := base(version)
		if err != nil {
			return nil, err
		}
		selected["core_version"] = normalized
		modules := make(map[string]string)
		if raw, ok := selected["modules"]; ok {
			pins, ok := raw.(map[string]any)
			if !ok {
				return nil, errors.New("invalid release module pins")
			}
			for module, raw := range pins {
				version, _ := raw.(string)
				normalized, err := base(version)
				if err != nil {
					return nil, err
				}
				modules[module] = normalized
			}
		}
		selected["modules"] = modules
	}
	return canonicalJSON(p)
}

var inlineRequire = regexp.MustCompile(`^([\t ]*require[\t ]+)([^\s]+)([\t ]+)(v[^\s]+)`)
var blockRequire = regexp.MustCompile(`^([\t ]*)([^\s]+)([\t ]+)(v[^\s]+)`)

func normalize(pathname string, content []byte) ([]byte, error) {
	if pathname == ".github/release-policy.json" {
		return normalizePolicy(content)
	}
	switch path.Base(pathname) {
	case "go.mod":
		var out strings.Builder
		block := false
		for _, line := range strings.SplitAfter(string(content), "\n") {
			withoutComment, _, _ := strings.Cut(line, "//")
			switch strings.TrimSpace(withoutComment) {
			case "require (":
				block = true
			case ")":
				block = false
			}
			pattern := inlineRequire
			if block {
				pattern = blockRequire
			}
			match := pattern.FindStringSubmatch(line)
			if match != nil && managed[match[2]] {
				if normalized, err := base(match[4]); err == nil {
					line = match[1] + match[2] + match[3] + normalized + line[len(match[0]):]
				}
			}
			out.WriteString(line)
		}
		return []byte(out.String()), nil
	case "go.sum":
		normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
		normalized = strings.ReplaceAll(normalized, "\r", "\n")
		var lines []string
		for _, line := range strings.Split(strings.TrimSuffix(normalized, "\n"), "\n") {
			module, _, _ := strings.Cut(line, " ")
			if !managed[module] {
				lines = append(lines, line)
			}
		}
		return []byte(strings.Join(lines, "\n")), nil
	}
	return content, nil
}

type API interface {
	API(string, string, any, any, bool) (bool, error)
}

func get(repo API, endpoint string, output any) error {
	_, err := repo.API("GET", endpoint, nil, output, false)
	return err
}

func Tree(repo API, sha string) (string, error) {
	var result struct{ Tree struct{ SHA string } }
	err := get(repo, "git/commits/"+sha, &result)
	if err == nil && result.Tree.SHA == "" {
		err = errors.New("commit tree SHA missing")
	}
	return result.Tree.SHA, err
}

func Fingerprint(repo API, sha string) (string, error) {
	tree, err := Tree(repo, sha)
	if err != nil {
		return "", err
	}
	var listing struct {
		Truncated bool
		Tree      []struct{ Path, Mode, Type, SHA string }
	}
	if err := get(repo, "git/trees/"+tree+"?recursive=1", &listing); err != nil {
		return "", err
	}
	if listing.Truncated || listing.Tree == nil {
		return "", errors.New("Failed to inspect the complete source tree")
	}
	records := make([][4]string, 0, len(listing.Tree))
	for _, entry := range listing.Tree {
		if entry.Type == "tree" {
			continue
		}
		digest := entry.SHA
		if entry.Path == ".github/release-policy.json" || path.Base(entry.Path) == "go.mod" || path.Base(entry.Path) == "go.sum" {
			var blob struct{ Content string }
			if err := get(repo, "git/blobs/"+digest, &blob); err != nil {
				return "", err
			}
			data, err := base64.StdEncoding.DecodeString(blob.Content)
			if err != nil {
				return "", err
			}
			normalized, err := normalize(entry.Path, data)
			if err != nil {
				return "", err
			}
			digest = fmt.Sprintf("%x", sha256.Sum256(normalized))
		}
		records = append(records, [4]string{entry.Path, entry.Mode, entry.Type, digest})
	}
	sort.Slice(records, func(i, j int) bool {
		for k, left := range records[i] {
			right := records[j][k]
			if left != right {
				return left < right
			}
		}
		return false
	})
	data, err := canonicalJSON(records)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}
