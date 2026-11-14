package publiccheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const Core = "github.com/NARUBROWN/spine"
const sharedMSA = "github.com/NARUBROWN/spine-simple-msa"

type Components struct {
	CoreVersion string            `json:"core_version"`
	Modules     map[string]string `json:"modules"`
}

type Plan struct {
	Module     string
	Version    string
	Kind       string
	SourceSHA  string `json:"source_sha"`
	Components json.RawMessage
}

type Report struct {
	Result     string            `json:"result"`
	Scope      string            `json:"scope"`
	SourceSHA  string            `json:"source_sha"`
	Version    string            `json:"version"`
	Components json.RawMessage   `json:"components"`
	Selected   map[string]string `json:"selected_modules"`
	GoVersion  string            `json:"go_version"`
	Evidence   []string          `json:"evidence"`
}

type Runner struct {
	Env     map[string]string
	Output  io.Writer
	command func(string, time.Duration, []string) ([]byte, []byte, error)
	sleep   func(time.Duration)
}

func NewRunner(env map[string]string, output io.Writer) Runner {
	runner := Runner{Env: env, Output: output, sleep: time.Sleep}
	runner.command = runner.run
	return runner
}

func Environment(work string) map[string]string {
	env := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	maps.Copy(env, map[string]string{
		"GOWORK": "off", "GOMODCACHE": filepath.Join(work, "modules"), "GOBIN": filepath.Join(work, "bin"), "GOFLAGS": "-modcacherw",
		"GOPROXY": "https://proxy.golang.org,direct", "GOSUMDB": "sum.golang.org", "GOPRIVATE": "", "GONOPROXY": "", "GONOSUMDB": "",
		"SPINE_KAFKA_BROKERS": env["SPINE_TEST_KAFKA_BROKER"], "SPINE_KAFKA_GROUP_ID": "release-verification", "SPINE_KAFKA_ALLOW_INSECURE": "true",
		"SPINE_RABBITMQ_URL": env["SPINE_TEST_RABBITMQ_URL"], "SPINE_RABBITMQ_EXCHANGE": "release-verification", "SPINE_RABBITMQ_ALLOW_INSECURE": "true",
	})
	return env
}

func (r Runner) run(dir string, timeout time.Duration, args []string) ([]byte, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	for key, value := range r.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var out, diagnostic bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &diagnostic
	err := cmd.Run()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return out.Bytes(), diagnostic.Bytes(), err
}

func (r Runner) capture(dir string, args ...string) ([]byte, error) {
	out, stderr, err := r.command(dir, 600*time.Second, args)
	if err != nil {
		return nil, fmt.Errorf("%s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(stderr)), err)
	}
	return out, nil
}

func (r Runner) Execute(dir string, args ...string) error {
	fmt.Fprintf(r.Output, "[%s] %s\n", filepath.Base(dir), strings.Join(args, " "))
	out, stderr, err := r.command(dir, 600*time.Second, args)
	r.Output.Write(out)
	r.Output.Write(stderr)
	return err
}

func (r Runner) Tests(dir string, args ...string) error {
	args = append([]string{"go", "test", "-json"}, args...)
	fmt.Fprintf(r.Output, "[%s] %s\n", filepath.Base(dir), strings.Join(args, " "))
	out, stderr, err := r.command(dir, 600*time.Second, args)
	r.Output.Write(out)
	r.Output.Write(stderr)
	if err != nil && errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	ran, passed, invalid := false, false, err != nil
	for line := range bytes.SplitSeq(out, []byte("\n")) {
		var event struct{ Action, Test string }
		if json.Unmarshal(line, &event) != nil {
			continue
		}
		switch event.Action {
		case "run":
			ran = ran || event.Test != ""
		case "pass":
			passed = passed || event.Test != ""
		case "skip":
			invalid = invalid || event.Test != ""
		case "fail":
			invalid = true
		}
	}
	if invalid || !ran || !passed {
		return errors.New("Public verification requires actual test execution with no failures or skips")
	}
	return nil
}

type GoModule struct {
	Path    string
	Version string
	Replace *struct{ Path string }
}

func (r Runner) Modules(dir string, expected map[string]string) (map[string]string, error) {
	raw, err := r.capture(dir, "go", "list", "-m", "-json", "all")
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	found := make(map[string]string)
	for {
		var item GoModule
		err := decoder.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if item.Replace != nil && item.Path != sharedMSA {
			return nil, fmt.Errorf("Public verification still contains replace directives: %s", item.Path)
		}
		if want, ok := expected[item.Path]; ok {
			if item.Version != want || item.Replace != nil {
				return nil, fmt.Errorf("Verification target version does not match: %s %s", item.Path, item.Version)
			}
			found[item.Path] = item.Version
		}
	}
	if len(found) != len(expected) {
		return nil, errors.New("The verification app does not use the candidate core")
	}
	return found, nil
}

type ModuleEdit struct {
	Require []GoModule
	Replace []struct{ Old GoModule }
}

func (r Runner) ReadModule(dir string) (ModuleEdit, error) {
	var result ModuleEdit
	data, err := r.capture(dir, "go", "mod", "edit", "-json")
	if err == nil {
		err = json.Unmarshal(data, &result)
	}
	return result, err
}

func (r Runner) pin(dir string, expected map[string]string) (map[string]string, error) {
	parsed, err := r.ReadModule(dir)
	if err != nil {
		return nil, err
	}
	for _, rule := range parsed.Replace {
		if rule.Old.Path != sharedMSA {
			if err := r.Execute(dir, "go", "mod", "edit", "-dropreplace="+rule.Old.Path); err != nil {
				return nil, err
			}
		}
	}
	pins := map[string]string{Core: expected[Core]}
	for _, req := range parsed.Require {
		if version, ok := expected[req.Path]; ok {
			pins[req.Path] = version
		}
	}
	if err := r.installPins(dir, pins); err != nil {
		return nil, err
	}
	return r.Modules(dir, pins)
}

func (r Runner) installPins(dir string, pins map[string]string) error {
	for _, module := range moduleOrder(pins) {
		if err := r.Execute(dir, "go", "mod", "edit", "-require="+module+"@"+pins[module]); err != nil {
			return err
		}
	}
	for _, step := range []string{"tidy", "verify"} {
		if err := r.Execute(dir, "go", "mod", step); err != nil {
			return err
		}
	}
	return nil
}

func moduleOrder(pins map[string]string) []string {
	modules := slices.Sorted(maps.Keys(pins))
	if i := slices.Index(modules, Core); i > 0 {
		copy(modules[1:i+1], modules[:i])
		modules[0] = Core
	}
	return modules
}

func (r Runner) consumer(work string, expected map[string]string) (map[string]string, error) {
	dir := filepath.Join(work, "consumer")
	if err := os.Mkdir(dir, 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/spine-release-check\n\ngo 1.26.0\n"), 0644); err != nil {
		return nil, err
	}
	var source strings.Builder
	source.WriteString("package main\nimport (\n")
	for _, module := range moduleOrder(expected) {
		fmt.Fprintf(&source, " _ %q\n", module)
	}
	source.WriteString(")\nfunc main() {}\n")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source.String()), 0644); err != nil {
		return nil, err
	}
	if err := r.installPins(dir, expected); err != nil {
		return nil, err
	}
	if err := r.Execute(dir, "go", "build", "-mod=readonly", "-o", filepath.Join(work, "consumer-app"), "."); err != nil {
		return nil, err
	}
	return r.Modules(dir, expected)
}

func (r Runner) waitForTag(module, version, work string) error {
	target := module + "@" + version
	for _, delay := range []time.Duration{15, 30, 60, 120, 0} {
		out, stderr, err := r.command(work, 60*time.Second, []string{"go", "mod", "download", "-json", target})
		if err == nil {
			return nil
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		message := string(out) + string(stderr)
		retry := strings.Contains(message, target) && strings.Contains(message, "verifying module") &&
			strings.Contains(message, "sum.golang.org/lookup/") && strings.Contains(message, "404 Not Found")
		if !retry || delay == 0 {
			return fmt.Errorf("Public module download failed for %s: %s", target, strings.TrimSpace(message))
		}
		fmt.Fprintf(r.Output, "Public checksum for %s is pending. Retrying in %ds.\n", target, delay)
		r.sleep(delay * time.Second)
	}
	return errors.New("public checksum retry exhausted")
}

func copyTree(source, target string) error {
	if source == "" {
		return errors.New("published module directory is missing")
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.Mkdir(dest, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0644)
	})
}

func (r Runner) Verify(plan Plan, source, work, query, scope string) (Report, error) {
	var report Report
	switch scope {
	case "core", "platform", "cohort":
	default:
		return report, errors.New("Specify the public verification scope")
	}
	if scope != "core" {
		for _, key := range []string{"SPINE_TEST_KAFKA_BROKER", "SPINE_TEST_RABBITMQ_URL"} {
			if r.Env[key] == "" {
				return report, fmt.Errorf("%s is required for broker E2E verification", key)
			}
		}
	}
	if scope == "cohort" && r.Env["SPINE_TEST_NATS_URL"] == "" {
		return report, errors.New("SPINE_TEST_NATS_URL is required for NATS cohort E2E verification")
	}
	if query == plan.Version {
		if err := r.waitForTag(plan.Module, query, work); err != nil {
			return report, err
		}
	}
	data, err := r.capture(work, "go", "list", "-m", "-json", plan.Module+"@"+query)
	if err != nil {
		return report, err
	}
	var resolved GoModule
	if err := json.Unmarshal(data, &resolved); err != nil {
		return report, err
	}
	if resolved.Version == "" {
		return report, errors.New("resolved public module version is missing")
	}
	var components Components
	if err := json.Unmarshal(plan.Components, &components); err != nil {
		return report, err
	}
	expected := map[string]string{Core: resolved.Version}
	switch scope {
	case "core":
		if plan.Kind != "core" {
			return report, errors.New("Core candidate verification applies only to the core")
		}
	case "platform":
		if plan.Kind != "platform" {
			return report, errors.New("Platform verification applies only to platforms")
		}
		expected = map[string]string{Core: components.CoreVersion, plan.Module: resolved.Version}
	case "cohort":
		if plan.Kind != "core" || resolved.Version != components.CoreVersion {
			return report, errors.New("The core version in the component versions does not match the plan")
		}
		maps.Copy(expected, components.Modules)
	}
	selected, err := r.consumer(work, expected)
	if err != nil {
		return report, err
	}
	testArgs := []string{"-mod=readonly", "-race", "./...", "-count=1", "-timeout=4m"}
	switch scope {
	case "platform":
		data, err := r.capture(filepath.Join(work, "consumer"), "go", "mod", "download", "-json", plan.Module+"@"+query)
		if err != nil {
			return report, err
		}
		var module struct{ Dir string }
		if err := json.Unmarshal(data, &module); err != nil {
			return report, err
		}
		tests := filepath.Join(work, "published-platform")
		if err := copyTree(module.Dir, tests); err != nil {
			return report, err
		}
		if err := r.Tests(tests, testArgs...); err != nil {
			return report, err
		}
		if err := r.Execute(tests, "go", "vet", "-mod=readonly", "./..."); err != nil {
			return report, err
		}
		if _, err := r.Modules(tests, map[string]string{Core: components.CoreVersion}); err != nil {
			return report, err
		}
	case "cohort":
		for _, name := range []string{"platform-e2e", "nats-e2e", "openapi", "http-platform"} {
			tests := filepath.Join(work, name)
			if err := copyTree(filepath.Join(source, "test", name), tests); err != nil {
				return report, err
			}
			pins, err := r.pin(tests, expected)
			if err != nil {
				return report, err
			}
			maps.Copy(selected, pins)
			if err := r.Tests(tests, testArgs...); err != nil {
				return report, err
			}
		}
	}
	version, err := r.capture("", "go", "version")
	if err != nil {
		return report, err
	}
	return Report{"passed", scope, plan.SourceSHA, query, plan.Components, selected, strings.TrimSpace(string(version)),
		[]string{"published modules without go.work or replace", "consumer build", "scope=" + scope}}, nil
}
