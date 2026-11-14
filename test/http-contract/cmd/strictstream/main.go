package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"
)

const expectedHash = "910be3224394de885c51cbea3aeb54eb72e38702f0a7d4c83a9e3207273a3777"

var expectedFailures = []string{
	"TestTruncatedRequestPreservesReadFailure/fasthttp/chunk-terminator",
	"TestTruncatedRequestPreservesReadFailure/fasthttp/trailer-terminator",
}

type testEvent struct {
	Test   string
	Action string
}

type stats struct {
	LeafPass    int      `json:"leaf_pass"`
	LeafFail    int      `json:"leaf_fail"`
	Skip        int      `json:"skip"`
	FailedCases []string `json:"failed_cases"`
	PackagePass int      `json:"package_pass"`
}

func summarize(output []byte) (stats, error) {
	result := stats{FailedCases: []string{}}
	var tests []testEvent
	parents := make(map[string]bool)
	for line := range bytes.SplitSeq(output, []byte("\n")) {
		if !bytes.HasPrefix(line, []byte("{")) {
			continue
		}
		var event testEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return result, err
		}
		if event.Test == "" {
			if event.Action == "pass" {
				result.PackagePass++
			}
			continue
		}
		switch event.Action {
		case "pass", "fail", "skip":
			tests = append(tests, event)
			for name := event.Test; strings.Contains(name, "/"); {
				name = name[:strings.LastIndexByte(name, '/')]
				parents[name] = true
			}
		}
	}
	for _, event := range tests {
		if event.Action == "skip" {
			result.Skip++
		}
		if parents[event.Test] {
			continue
		}
		switch event.Action {
		case "pass":
			result.LeafPass++
		case "fail":
			result.LeafFail++
			result.FailedCases = append(result.FailedCases, event.Test)
		}
	}
	slices.Sort(result.FailedCases)
	result.FailedCases = slices.Compact(result.FailedCases)
	return result, nil
}

func digest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

func snapshot(root string, names []string) (map[string]string, error) {
	result := make(map[string]string, len(names))
	for _, name := range names {
		value, err := digest(filepath.Join(root, name))
		if err != nil {
			return nil, err
		}
		result[name] = value
	}
	return result, nil
}

func testInputs(root string) (map[string]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.go") {
			names = append(names, entry.Name())
		}
	}
	return snapshot(root, names)
}

func unchanged(root string, original map[string]string) bool {
	names := make([]string, 0, len(original))
	for name := range original {
		names = append(names, name)
	}
	current, err := snapshot(root, names)
	return err == nil && reflect.DeepEqual(original, current)
}

func command(ctx context.Context, dir, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), "GOWORK=off")
	return cmd
}

func readCommand(dir, name string, args ...string) ([]byte, error) {
	cmd := command(context.Background(), dir, name, args...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

type stepResult struct {
	ExitCode int `json:"exit_code"`
	*stats
}

func runStep(dir, out, name string, results map[string]any, args ...string) (stepResult, error) {
	var result stepResult
	if err := json.NewEncoder(os.Stdout).Encode(map[string]string{"step": name, "status": "running"}); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := command(ctx, dir, "go", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return result, err
		}
		result.ExitCode = exit.ExitCode()
	}
	if err := os.WriteFile(filepath.Join(out, name+".stdout"), stdout.Bytes(), 0644); err != nil {
		return result, err
	}
	if err := os.WriteFile(filepath.Join(out, name+".stderr"), stderr.Bytes(), 0644); err != nil {
		return result, err
	}
	if slices.Contains(args, "-json") {
		summary, err := summarize(stdout.Bytes())
		if err != nil {
			return result, err
		}
		result.stats = &summary
	}
	results[name] = result
	return result, json.NewEncoder(os.Stdout).Encode(struct {
		Step string `json:"step"`
		stepResult
	}{name, result})
}

func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == ".git" || entry.Name() == "__pycache__" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.Mkdir(dest, 0700)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.Create(dest)
		if err != nil {
			return err
		}
		_, err = io.Copy(output, input)
		return errors.Join(err, output.Close())
	})
}

func compare(module, core, out string) (passed bool, err error) {
	raw, err := readCommand(module, "go", "list", "-m", "-json", "github.com/valyala/fasthttp")
	if err != nil {
		return false, err
	}
	var dep struct {
		Version, Dir string
		Replace      json.RawMessage
	}
	if err := json.Unmarshal(raw, &dep); err != nil {
		return false, err
	}
	if dep.Version != "v1.77.0" || (len(dep.Replace) > 0 && string(dep.Replace) != "null") {
		return false, errors.New("순정 fasthttp v1.77.0에서 시작해야 합니다.")
	}
	original := filepath.Join(dep.Dir, "streaming.go")
	originalHash, err := digest(original)
	if err != nil {
		return false, err
	}
	if originalHash != expectedHash {
		return false, errors.New("고정한 원본 streaming.go 해시와 다릅니다.")
	}
	tests, err := testInputs(module)
	if err != nil {
		return false, err
	}
	var inputs []string
	for _, name := range []string{"go.mod", "go.sum"} {
		if _, err := os.Stat(filepath.Join(core, name)); err == nil {
			inputs = append(inputs, name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	coreInputs, err := snapshot(core, inputs)
	if err != nil {
		return false, err
	}
	contract, err := filepath.Glob(filepath.Join(core, "contract", "http", "*.go"))
	if err != nil {
		return false, err
	}
	if len(contract) == 0 {
		return false, errors.New("정식 HTTP 계약 소스가 없습니다.")
	}
	for i, path := range contract {
		contract[i], err = filepath.Rel(core, path)
		if err != nil {
			return false, err
		}
	}
	contractInputs, err := snapshot(core, contract)
	if err != nil {
		return false, err
	}
	version, err := readCommand(module, "go", "version")
	if err != nil {
		return false, err
	}
	patch := filepath.Join(module, "patches", "fasthttp-v1.77.0-strict-chunk-eof.patch")
	patchHash, err := digest(patch)
	if err != nil {
		return false, err
	}
	results := map[string]any{"fasthttp_version": dep.Version, "go_version": strings.TrimSpace(string(version)), "patch_sha256": patchHash, "original_streaming_sha256": originalHash, "test_sha256": tests}
	temp, err := os.MkdirTemp("", "spine-strict-stream-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(temp)
	if err := os.MkdirAll(out, 0755); err != nil {
		return false, err
	}
	defer func() {
		current, currentErr := testInputs(module)
		cacheIntact := unchanged(dep.Dir, map[string]string{"streaming.go": originalHash})
		contractIntact := unchanged(core, contractInputs)
		coreIntact := unchanged(core, coreInputs)
		fixtureIntact := currentErr == nil && reflect.DeepEqual(tests, current)
		results["module_cache_unchanged"] = cacheIntact
		results["contract_unchanged"] = contractIntact
		results["core_module_inputs_unchanged"] = coreIntact
		results["fixture_tests_unchanged"] = fixtureIntact
		data, marshalErr := json.MarshalIndent(results, "", "  ")
		if marshalErr == nil {
			marshalErr = os.WriteFile(filepath.Join(out, "summary.json"), append(data, '\n'), 0644)
		}
		err = errors.Join(err, marshalErr)
		if err == nil {
			intact := cacheIntact && contractIntact && coreIntact && fixtureIntact
			err = json.NewEncoder(os.Stdout).Encode(map[string]bool{"success": passed, "inputs_intact": intact})
			passed = passed && intact
		}
	}()
	full := []string{"test", "-mod=readonly", "-race", "-json", "-count=1", "-timeout=90s", "./..."}
	baseline, err := runStep(module, out, "baseline", results, full...)
	if err != nil {
		return false, err
	}
	if baseline.ExitCode != 1 || !slices.Equal(baseline.FailedCases, expectedFailures) {
		return false, errors.New("기준 실행이 알려진 두 실패와 다릅니다. 로그를 확인해야 합니다.")
	}
	engine := filepath.Join(temp, "fasthttp")
	if err := copyTree(dep.Dir, engine); err != nil {
		return false, err
	}
	applied, err := command(context.Background(), engine, "git", "apply", "--check", "--unidiff-zero", patch).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("분리 복사본에 패치를 적용할 수 없습니다: %s", applied)
	}
	if _, err := readCommand(engine, "git", "apply", "--unidiff-zero", patch); err != nil {
		return false, err
	}
	results["patched_streaming_sha256"], err = digest(filepath.Join(engine, "streaming.go"))
	if err != nil {
		return false, err
	}
	fixture := filepath.Join(temp, "probe")
	if err := copyTree(module, fixture); err != nil {
		return false, err
	}
	if _, err := readCommand(fixture, "go", "mod", "edit", "-replace=github.com/NARUBROWN/spine="+core, "-replace=github.com/valyala/fasthttp="+engine); err != nil {
		return false, err
	}
	copied, err := testInputs(fixture)
	if err != nil || !reflect.DeepEqual(tests, copied) {
		return false, errors.New("패치 전후의 테스트가 달라졌습니다.")
	}
	success := true
	for _, step := range []struct {
		dir, name string
		args      []string
	}{
		{fixture, "patched", full},
		{fixture, "repeated", []string{"test", "-mod=readonly", "-race", "-json", "-count=20", "-timeout=60s", "-run=TestTruncatedRequestPreservesReadFailure/^fasthttp$/(chunk-terminator|trailer-terminator)$", "./..."}},
		{fixture, "vet", []string{"vet", "-mod=readonly", "./..."}},
		{engine, "upstream_stream_tests", []string{"test", "-mod=readonly", "-race", "-json", "-count=1", "-timeout=90s", "-run=Test.*(Stream|Chunk|Trailer).*", "."}},
	} {
		result, err := runStep(step.dir, out, step.name, results, step.args...)
		if err != nil {
			return false, err
		}
		success = success && result.ExitCode == 0
	}
	copied, err = testInputs(fixture)
	if err != nil {
		return false, err
	}
	results["success"] = success
	results["same_tests_before_after"] = reflect.DeepEqual(tests, copied)
	return success, nil
}

func main() {
	output := flag.String("output-dir", "", "output directory")
	flag.Parse()
	if *output == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "--output-dir is required")
		os.Exit(2)
	}
	module, err := os.Getwd()
	if err == nil {
		var passed bool
		passed, err = compare(module, filepath.Join(module, "..", ".."), *output)
		if err == nil && passed {
			return
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(1)
}
