package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, root, name, contents string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunnerExecutesPrivateTestsAndReportsFailuresWithoutWritingSourceFiles(t *testing.T) {
	t.Setenv("GOWORK", "off")
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module runnerfixture\n\ngo 1.25.5\n")
	source := writeFixture(t, root, "value.go", "package runnerfixture\nfunc privateValue() int { return 42 }\n")
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		value string
		code  int
		want  string
	}{{"42", 0, "--- PASS: TestPrivate"}, {"0", 1, "--- FAIL: TestPrivate"}} {
		writeFixture(t, root, "test/unit/value_test.go", "package runnerfixture\nimport \"testing\"\nfunc TestPrivate(t *testing.T) { if privateValue() != "+test.value+" { t.Fatal(\"value mismatch\") } }\n")
		var output bytes.Buffer
		if code := run([]string{"-root", root, "test", ".", "-count=1", "-v"}, &output, &output); code != test.code || !strings.Contains(output.String(), test.want) {
			t.Fatalf("exit=%d want=%d: %s", code, test.code, &output)
		}
		if _, err := os.Stat(filepath.Join(root, "value_test.go")); !os.IsNotExist(err) {
			t.Fatalf("Test files must not be written to source directories: %v", err)
		}
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatalf("Source code changed: %v", err)
	}
}

func TestRunnerRejectsColocatedAndOrphanedTests(t *testing.T) {
	for _, name := range []string{"value_test.go", "internal/example/test/value_test.go"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFixture(t, root, name, "package example\n")
			if _, err := testFiles(root); err == nil || !strings.Contains(err.Error(), "test/unit") {
				t.Fatalf("Tests outside the root test directory must be rejected: %v", err)
			}
		})
	}
	root := t.TempDir()
	writeFixture(t, root, "test/unit/missing/value_test.go", "package missing\n")
	if _, err := testFiles(root); err == nil || !strings.Contains(err.Error(), "Source package") {
		t.Fatalf("Tests without a source package must be rejected: %v", err)
	}
}

func TestRunnerMapsNestedModulesAndIgnoresIntegrationTests(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example\n\ngo 1.25.5\n")
	writeFixture(t, root, "cmd/demo/go.mod", "module example/demo\n\ngo 1.25.5\n")
	file := writeFixture(t, root, "test/unit/cmd/demo/main_test.go", "package main\n")
	writeFixture(t, root, "test/app_integration_test.go", "package test\n")
	canonical, module, err := paths(root, "cmd/demo")
	if err != nil || module != filepath.Join(canonical, "cmd/demo") {
		t.Fatalf("Nested module path: %s %v", module, err)
	}
	files, err := testFiles(root)
	if err != nil || len(files) != 1 || files[filepath.Join(root, "cmd/demo/main_test.go")] != file {
		t.Fatalf("Nested module mapping: %+v %v", files, err)
	}
}

func TestRunnerRejectsMissingModulesAndDirectoryEscapes(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example\n\ngo 1.25.5\n")
	for _, module := range []string{"../", root, "missing"} {
		if _, _, err := paths(root, module); err == nil {
			t.Fatalf("Invalid module path must be rejected: %s", module)
		}
	}
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Windows checks that require symbolic link creation privileges")
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	canonical, _, err := paths(alias, ".")
	want, resolveErr := filepath.EvalSymlinks(root)
	if err != nil || resolveErr != nil || canonical != want {
		t.Fatalf("Symbolic link path resolution: %s %v %v", canonical, err, resolveErr)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := paths(root, "escape"); err == nil {
		t.Fatal("Symbolic links must not point outside the repository")
	}
}

func TestRunnerRejectsCommandsThatBypassTheOverlay(t *testing.T) {
	for _, args := range [][]string{nil, {"build", "./..."}, {"test", "-overlay=other.json"}, {"test", "-C", "other"}} {
		var output bytes.Buffer
		if code := run(args, &output, &output); code != 2 {
			t.Fatalf("Invalid command exit code: %v = %d: %s", args, code, &output)
		}
	}
}
