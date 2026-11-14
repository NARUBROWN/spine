// test는 루트 test/unit의 테스트를 원본 패키지에서 실행합니다.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "Repository root")
	module := flags.String("module", ".", "Go module to run, relative to the repository root")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: go run ./scripts/test [-module path] {test|vet|list} [Go arguments...]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	command := flags.Args()
	if len(command) == 0 || (command[0] != "test" && command[0] != "vet" && command[0] != "list") {
		flags.Usage()
		return 2
	}
	for _, arg := range command[1:] {
		if arg == "-overlay" || strings.HasPrefix(arg, "-overlay=") || arg == "-C" || strings.HasPrefix(arg, "-C=") {
			fmt.Fprintln(stderr, "The runner manages -overlay and -C. Select a module with -module.")
			return 2
		}
	}
	rootPath, modulePath, err := paths(*root, *module)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	replacements, err := testFiles(rootPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	temp, err := os.MkdirTemp("", "spine-tests-*")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer os.RemoveAll(temp)
	overlay := filepath.Join(temp, "overlay.json")
	data, err := json.Marshal(struct {
		Replace map[string]string
	}{replacements})
	if err == nil {
		err = os.WriteFile(overlay, data, 0o600)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	goArgs := append([]string{command[0], "-overlay=" + overlay}, command[1:]...)
	cmd := exec.Command("go", goArgs...)
	cmd.Dir = modulePath
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func paths(root, module string) (string, string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	// macOS의 /tmp처럼 심볼릭 링크인 경로도 Go가 사용하는 실제 경로와 맞춥니다.
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", err
	}
	if filepath.IsAbs(module) {
		return "", "", fmt.Errorf("-module requires a path relative to the repository root: %s", module)
	}
	target, err := filepath.EvalSymlinks(filepath.Join(root, module))
	if err != nil {
		return "", "", err
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("Module path points outside the repository: %s", module)
	}
	if _, err := os.Stat(filepath.Join(target, "go.mod")); err != nil {
		return "", "", fmt.Errorf("Go module not found: %w", err)
	}
	return root, target, nil
}

func testFiles(root string) (map[string]string, error) {
	// 제품 테스트는 test/unit에 두고, 검증 도구의 테스트는 도구 패키지에 둡니다.
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path == filepath.Join(root, "test") || path == filepath.Join(root, "scripts") || (path != root && (entry.Name() == "vendor" || strings.HasPrefix(entry.Name(), "."))) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			return fmt.Errorf("Move test files under test/unit at the repository root: %s", path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	unit := filepath.Join(root, "test", "unit")
	replacements := make(map[string]string)
	err = filepath.WalkDir(unit, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("Test files must be regular files: %s", path)
		}
		relative, err := filepath.Rel(unit, path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, relative)
		if _, err := os.Stat(filepath.Dir(target)); err != nil {
			return fmt.Errorf("Source package directory for test not found: %w", err)
		}
		replacements[target] = path
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(replacements) == 0 {
		return nil, fmt.Errorf("No runnable tests found under test/unit")
	}
	return replacements, nil
}
