package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinAndCheckPlatform(t *testing.T) {
	root := t.TempDir()
	module := core + "-echo-platform"
	original := "module " + module + "\n\ngo 1.26.0\n\nrequire " + core + " v0.7.0-preview.1\n"
	path := filepath.Join(root, "go.mod")
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	p := policy{Kind: "platform", Module: module, Channels: map[string]selection{
		"rc": {Core: "v0.7.0-rc.1"}, "preview": {Core: "v0.7.0-preview.1"},
	}}
	var output bytes.Buffer
	if err := pin(root, p, "rc", false, false, &output); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != original {
		t.Fatalf("dry-run go.mod = %q, %v", got, err)
	}
	if !strings.Contains(output.String(), "+require "+core+" v0.7.0-rc.1") {
		t.Fatal(output.String())
	}
	applied := bytes.Buffer{}
	if err := pin(root, p, "rc", true, false, &applied); err != nil {
		t.Fatal(err)
	}
	if applied.String() != output.String() {
		t.Fatalf("dry-run and apply differ: %s", applied.String())
	}
	if err := checkModule(root, p, "rc"); err != nil {
		t.Fatal(err)
	}
	if err := checkModule(root, p, "preview"); err == nil {
		t.Fatal("wrong pin accepted")
	}
}

func TestCheckCoreWithEmptyRequirements(t *testing.T) {
	root := t.TempDir()
	for _, dir := range append([]string{""}, childModules...) {
		path := filepath.Join(root, dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		name := core
		if dir != "" {
			name = "example.com/child"
		}
		if err := os.WriteFile(filepath.Join(path, "go.mod"), []byte("module "+name+"\n\ngo 1.26.0\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p := policy{Kind: "core", Module: core, Channels: map[string]selection{"preview": {Core: "v0.7.0-preview.1"}}}
	if err := checkModule(root, p, "preview"); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, childModules[0], "go.mod")
	if err := os.WriteFile(child, []byte("module example.com/child\n\ngo 1.26.0\n\nrequire "+core+" v0.6.5\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkModule(root, p, "preview"); err == nil {
		t.Fatal("wrong child pin accepted")
	}
}
