package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, path, text string) {
	t.Helper()
	target := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDocumentationLayoutAndLinks(t *testing.T) {
	root := t.TempDir()
	write(t, root, "README.md", "[Guide](docs/guide.md#request-lifetime)\n")
	write(t, root, "AGENTS.md", "[Rules](.AGENT.md)\n")
	write(t, root, ".AGENT.md", "# Working rules\n")
	write(t, root, "docs/guide.md", "# Request lifetime\n[Source](../app.go)\n[한국어](ko/guide.md#요청-수명)\n```go\n// [not a link](missing.go)\n```\n")
	write(t, root, "docs/ko/guide.md", "# 요청 수명\n")
	write(t, root, "app.go", "package example\n")
	if err := verify(root); err != nil {
		t.Fatal(err)
	}
	write(t, root, "pkg/README.md", "# Unexpected package README\n")
	if err := verify(root); err == nil || !strings.Contains(err.Error(), "Markdown must live under docs/") {
		t.Fatalf("unexpected layout result: %v", err)
	}
}

func TestBrokenLinksAndAnchors(t *testing.T) {
	for _, link := range []string{"missing.md", "guide.md#missing", "../../outside.md"} {
		t.Run(link, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, "docs/index.md", "[Broken]("+link+")\n")
			write(t, root, "docs/guide.md", "# Existing\n")
			if err := verify(root); err == nil {
				t.Fatal("broken link accepted")
			}
		})
	}
}
