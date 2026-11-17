package main

import (
	"bytes"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, name, value string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSourcesRejectPythonFilesAndWorkflowCommands(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		ok            bool
	}{
		{"scripts/check.go", "package main", true}, {"scripts/check.py", "", false}, {".github/actions/tool/check.py", "", false},
		{".github/workflows/ci.yml", "run: python3 check.py", false}, {".github/workflows/ci.yaml", "run: python3.12 check.py", false},
		{".github/actions/tool/action.yml", "run: python.exe check.py", false},
		{".github/workflows/ci.yml", "run: 'python3 -c print(1)'", false},
		{".github/workflows/ci.yml", "run: \"python -c print(1)\"", false},
		{".github/workflows/ci.yml", "# python3 old.py\nrun: go test ./...", true},
		{"node_modules/package/check.py", "", true}, {".git/hooks/check.py", "", true}, {"__pycache__/generated.py", "", true},
	} {
		t.Run(tc.name+"/"+tc.content, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, tc.name, tc.content)
			if err := sources(root); (err == nil) != tc.ok {
				t.Fatalf("sources = %v, want success %t", err, tc.ok)
			}
		})
	}
	if err := sources(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing repository accepted")
	}
}

func TestCoverageUsesStatementWeightsAndMergesDuplicateBlocks(t *testing.T) {
	for _, tc := range []struct {
		profile       string
		minimum, want float64
		ok            bool
	}{
		{"mode: atomic\na.go:1.1,2.2 8 1\nb.go:1.1,2.2 2 0\n", 80, 80, true},
		{"mode: atomic\na.go:1.1,2.2 8 1\nb.go:1.1,2.2 2 0\n", 81, 80, false},
		{"mode: count\na.go:1.1,2.2 8 0\na.go:1.1,2.2 8 2\nb.go:1.1,2.2 2 0\n", 80, 80, true},
		{"mode: set\na.go:1.1,2.2 1 1\nb.go:1.1,2.2 9 0\n", 80, 10, false},
	} {
		got, err := coverage(strings.NewReader(tc.profile), tc.minimum)
		if got != tc.want || (err == nil) != tc.ok {
			t.Fatalf("coverage = %v, %v; want %v, success %t", got, err, tc.want, tc.ok)
		}
	}
}

func TestCoverageRejectsEmptyMalformedAndInconsistentProfiles(t *testing.T) {
	for _, profile := range []string{"", "invalid\n", "mode: atomic\n", "mode: atomic\na 0 1\n", "mode: atomic\ninvalid\n", "mode: atomic\na -1 1\n", "mode: atomic\na 1 -1\n", "mode: atomic\na 1 0\na 2 1\n", "mode: atomic\na 18446744073709551615 1\nb 1 1\n"} {
		if _, err := coverage(strings.NewReader(profile), 80); err == nil {
			t.Fatalf("accepted profile %q", profile)
		}
	}
	for _, minimum := range []float64{-1, 101, math.NaN(), math.Inf(1)} {
		if _, err := coverage(strings.NewReader("mode: atomic\na 1 1\n"), minimum); err == nil {
			t.Fatal("invalid threshold accepted")
		}
	}
}

func TestRunChecksSourcesAndCoverageTogether(t *testing.T) {
	root := t.TempDir()
	profile := write(t, root, "coverage.out", "mode: atomic\na.go:1.1,2.2 8 1\nb.go:1.1,2.2 2 0\n")
	var output bytes.Buffer
	if err := run([]string{"--root", root, "--coverage", profile}, &output); err != nil || !strings.Contains(output.String(), "80.00%") {
		t.Fatalf("%v: %s", err, &output)
	}
	if err := run([]string{"--root", root}, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--root", root, "unexpected"}, {"--unknown"}, {"--root", root, "--coverage", "missing"}, {"--root", root, "--coverage", profile, "--minimum", "81"}} {
		if err := run(args, io.Discard); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	write(t, root, "check.py", "")
	if err := run([]string{"--root", root}, io.Discard); err == nil {
		t.Fatal("Python source accepted")
	}
}

func TestRepositoryUsesGoMaintenanceSources(t *testing.T) {
	if err := sources(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
}
