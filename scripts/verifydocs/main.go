// verifydocs checks Markdown placement and repository-local links.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var markdownLink = regexp.MustCompile(`\]\(([^\s)]+)(?:\s+"[^"]*")?\)`)
var htmlLink = regexp.MustCompile(`(?:href|src)="([^"\s]+)"`)
var heading = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*#*\s*$`)

func visibleLines(data []byte) []string {
	var result []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	fence := ""
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			marker := trimmed[:3]
			if fence == "" {
				fence = marker
			} else if fence == marker {
				fence = ""
			}
			continue
		}
		if fence == "" {
			result = append(result, line)
		}
	}
	return result
}

func anchors(data []byte) map[string]bool {
	result := map[string]bool{}
	counts := map[string]int{}
	for _, line := range visibleLines(data) {
		found := heading.FindStringSubmatch(line)
		if found == nil {
			continue
		}
		slug := strings.Map(func(r rune) rune {
			switch {
			case unicode.IsSpace(r):
				return '-'
			case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
				return unicode.ToLower(r)
			default:
				return -1
			}
		}, found[1])
		base := slug
		if n := counts[base]; n > 0 {
			slug = fmt.Sprintf("%s-%d", base, n)
		}
		counts[base]++
		result[slug] = true
	}
	return result
}

func verify(root string) error {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	var documents []string
	var failures []error
	err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != absolute && (entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == "vendor" || strings.HasPrefix(entry.Name(), ".") && entry.Name() != ".github") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		relative, err := filepath.Rel(absolute, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !strings.HasPrefix(relative, "docs/") && relative != "README.md" && relative != "AGENTS.md" && relative != ".AGENT.md" {
			failures = append(failures, fmt.Errorf("Markdown must live under docs/: %s", relative))
		}
		documents = append(documents, path)
		return nil
	})
	if err != nil {
		failures = append(failures, err)
	}
	for _, path := range documents {
		data, err := os.ReadFile(path)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for _, line := range visibleLines(data) {
			links := append(markdownLink.FindAllStringSubmatch(line, -1), htmlLink.FindAllStringSubmatch(line, -1)...)
			for _, link := range links {
				raw := strings.Trim(link[1], "<>")
				parsed, err := url.Parse(raw)
				if err != nil {
					failures = append(failures, fmt.Errorf("%s: invalid link %q", path, raw))
					continue
				}
				if parsed.Scheme != "" || parsed.Host != "" {
					continue
				}
				target := path
				if parsed.Path != "" {
					if strings.HasPrefix(parsed.Path, "/") {
						target = filepath.Join(absolute, parsed.Path[1:])
					} else {
						target = filepath.Join(filepath.Dir(path), parsed.Path)
					}
				}
				relative, err := filepath.Rel(absolute, target)
				if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
					failures = append(failures, fmt.Errorf("%s: link escapes repository: %s", path, raw))
					continue
				}
				if _, err := os.Stat(target); err != nil {
					failures = append(failures, fmt.Errorf("%s: missing link target: %s", path, raw))
					continue
				}
				if parsed.Fragment != "" && strings.EqualFold(filepath.Ext(target), ".md") {
					linked, err := os.ReadFile(target)
					if err != nil {
						failures = append(failures, err)
						continue
					}
					if !anchors(linked)[parsed.Fragment] {
						failures = append(failures, fmt.Errorf("%s: missing Markdown anchor: %s", path, raw))
					}
				}
			}
		}
	}
	return errors.Join(failures...)
}

func main() {
	if err := verify("."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Markdown layout and local links verified")
}
