package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var childModules = []string{"test", "test/openapi", "test/platform-e2e", "test/nats-e2e", "test/http-platform", "cmd/demo", "cmd/reference", "examples/security-config"}

type moduleFile struct {
	Module  struct{ Path string }
	Require []struct{ Path, Version string }
	Replace []json.RawMessage
}

func moduleCommand(dir, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), "GOWORK=off")
	cmd.Stderr = os.Stderr
	return cmd
}

func readModule(dir string) (moduleFile, error) {
	var parsed moduleFile
	data, err := moduleCommand(dir, "go", "mod", "edit", "-json").Output()
	if err != nil {
		return parsed, err
	}
	err = json.Unmarshal(data, &parsed)
	return parsed, err
}

func pinnedVersion(s selection, module string) string {
	if module == core {
		return s.Core
	}
	return s.Modules[module]
}

func checkModule(root string, p policy, channel string) error {
	parsed, err := readModule(root)
	if err != nil {
		return err
	}
	if parsed.Module.Path != p.Module || len(parsed.Replace) != 0 {
		return errors.New("Published module path does not match or local replace directives remain")
	}
	selected := p.Channels[channel]
	if p.Kind == "platform" {
		for _, r := range parsed.Require {
			if r.Path == core && r.Version == selected.Core {
				return nil
			}
		}
		return errors.New("The core version in go.mod does not match the pinned version for the publication channel")
	}
	for _, r := range parsed.Require {
		if r.Path == core || knownPlatform(r.Path) {
			return errors.New("The core module must not depend on platform modules")
		}
	}
	for _, name := range childModules {
		child, err := readModule(filepath.Join(root, name))
		if err != nil {
			return err
		}
		for _, r := range child.Require {
			if expected := pinnedVersion(selected, r.Path); expected != "" && expected != r.Version {
				return fmt.Errorf("%s/go.mod does not match publication channel dependencies: %s", name, r.Path)
			}
		}
	}
	return nil
}

func pin(root string, p policy, channel string, apply, tidy bool, output io.Writer) error {
	paths := []string{root}
	if p.Kind == "core" {
		for _, name := range childModules {
			paths = append(paths, filepath.Join(root, name))
		}
	}
	for _, dir := range paths {
		if err := pinModule(dir, p.Channels[channel], apply, output); err != nil {
			return err
		}
		if apply && tidy {
			for _, step := range []string{"tidy", "verify"} {
				cmd := moduleCommand(dir, "go", "mod", step)
				cmd.Stdout = os.Stdout
				if err := cmd.Run(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func pinModule(dir string, selected selection, apply bool, output io.Writer) error {
	path := filepath.Join(dir, "go.mod")
	original, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	parsed, err := readModule(dir)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "spine-channel-pins-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	modfile := filepath.Join(tmp, "go.mod")
	if err := os.WriteFile(modfile, original, 0600); err != nil {
		return err
	}
	for _, requirement := range parsed.Require {
		if version := pinnedVersion(selected, requirement.Path); version != "" {
			if err := moduleCommand(dir, "go", "mod", "edit", "-modfile="+modfile, "-require="+requirement.Path+"@"+version).Run(); err != nil {
				return err
			}
		}
	}
	updated, err := os.ReadFile(modfile)
	if err != nil || bytes.Equal(original, updated) {
		return err
	}
	cmd := moduleCommand(dir, "git", "--no-pager", "diff", "--no-index", "--no-color", "--no-ext-diff", "--text", "--", path, modfile)
	diff, err := cmd.Output()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		return fmt.Errorf("generate module diff: %v", err)
	}
	_, hunks, found := strings.Cut(string(diff), "\n@@ ")
	if !found {
		return errors.New("module diff is missing a hunk")
	}
	lines := strings.SplitAfter("@@ "+hunks, "\n")
	for index, line := range lines {
		if strings.HasPrefix(line, "@@ ") {
			rangeHeader, _, _ := strings.Cut(line[3:], " @@")
			lines[index] = "@@ " + rangeHeader + " @@\n"
		}
	}
	if _, err := fmt.Fprintf(output, "--- %s\n+++ %s\n%s", path, path, strings.Join(lines, "")); err != nil {
		return err
	}
	if apply {
		return os.WriteFile(path, updated, 0666)
	}
	return nil
}

func checkPublicationDocs(root string, p policy) error {
	if p.Kind == "core" {
		bootstrap, err := os.ReadFile(filepath.Join(root, "internal", "bootstrap", "bootstrap.go"))
		if err != nil {
			return err
		}
		if !strings.Contains(string(bootstrap), fmt.Sprintf("spineVersion = %q", p.Next)) {
			return errors.New("Runtime development base version does not match next_version")
		}
	}
	info, err := os.Stat(filepath.Join(root, "docs", "migration", p.Next+".md"))
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("A migration guide for the target version is required")
	}
	names := []string{"README.md"}
	if p.Kind == "core" {
		names = append(names, "README.en.md", "README.ja.md", "README.zh-Hans.md", "README.zh-Hant.md")
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		if !strings.Contains(string(data), string(rune(96))+p.Next+string(rune(96))) {
			return fmt.Errorf("%s: check the target version", name)
		}
	}
	fmt.Println("Release policy format verified")
	return nil
}
