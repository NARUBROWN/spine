package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestChannelFor(t *testing.T) {
	for ref, want := range map[string]string{
		"preview": "preview", "rc": "rc", "main": "stable",
		"v0.7.0-preview.1": "preview", "v0.7.0-rc.1": "rc", "v0.7.0": "stable",
		"validator-fix": "preview", "version-bump": "preview", "v-next": "preview",
	} {
		if got := channelFor(ref); got != want {
			t.Errorf("%s: got %s, want %s", ref, got, want)
		}
	}
}

func TestSourceWorkspaceOffline(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "spine-core")
	if err := os.Mkdir(src, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "module " + core + "-echo-platform\n\ngo 1.26.0\n\nrequire " + core + " v0.7.0-preview.1\n"
	for path, data := range map[string]string{
		"go.mod":               manifest,
		"source.go":            "package platform\nimport spine \"" + core + "\"\nconst Value=spine.Value\n",
		"spine-core/go.mod":    "module " + core + "\n\ngo 1.26.0\n",
		"spine-core/source.go": "package spine\nconst Value=1\n",
	} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "source.work")
	envOutput := filepath.Join(root, "github.env")
	t.Setenv("GITHUB_ENV", envOutput)
	p := policy{Kind: "platform", Channels: map[string]selection{
		"preview": {Core: "v0.7.0-preview.1"}, "rc": {Core: "v0.7.0-rc.1"}, "stable": {Core: "v0.7.0"},
	}}
	if err := prepare(root, output, p, false); err != nil {
		t.Fatal(err)
	}
	wantWorkspace := fmt.Sprintf("go 1.26.0\n\nuse (\n\t%q\n\t%q\n)\n\nreplace (\n", root, filepath.Join(root, "spine-core"))
	for _, version := range []string{"v0.7.0-preview.1", "v0.7.0-rc.1", "v0.7.0"} {
		wantWorkspace += fmt.Sprintf("\t%s %s => %q\n", core, version, filepath.Join(root, "spine-core"))
	}
	wantWorkspace += ")\n"
	if got, err := os.ReadFile(output); err != nil || string(got) != wantWorkspace {
		t.Fatalf("workspace: %v, %q, want %q", err, got, wantWorkspace)
	}

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = root
	cmd.Env = append(cmd.Environ(), "GOWORK="+output, "GOPROXY=off", "GOSUMDB=off")
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("offline build: %v\n%s", err, data)
	}
	for path, want := range map[string]string{filepath.Join(root, "go.mod"): manifest, envOutput: "GOWORK=" + output + "\n"} {
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Fatalf("%s: %v, %q", path, err, got)
		}
	}
	if err := prepare(root, output, p, true); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(output); err != nil || strings.Contains(string(got), "replace") {
		t.Fatalf("published workspace: %v, %q", err, got)
	}
}
