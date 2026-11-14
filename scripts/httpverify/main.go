package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
)

type testKey struct{ Package, Test string }

func summarize(r io.Reader) (map[string]int, error) {
	tests := make(map[testKey]string)
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadBytes('\n')
		var event struct{ Package, Test, Action string }
		if json.Unmarshal(line, &event) == nil && event.Test != "" {
			switch event.Action {
			case "pass", "fail", "skip":
				tests[testKey{event.Package, event.Test}] = event.Action
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	parents := make(map[testKey]bool)
	for key := range tests {
		for name := key.Test; strings.Contains(name, "/"); {
			name = name[:strings.LastIndexByte(name, '/')]
			parents[testKey{key.Package, name}] = true
		}
	}
	counts := map[string]int{"pass": 0, "fail": 0, "skip": 0}
	for key, action := range tests {
		if !parents[key] {
			counts[action]++
		}
	}
	return counts, nil
}

type module struct{ name, root string }
type runner func(root string, env []string, output io.Writer, args ...string) (int, error)

func runGo(root string, env []string, output io.Writer, args ...string) (int, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir, cmd.Env = root, env
	cmd.Stdout, cmd.Stderr = output, output
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	return 0, err
}

func snapshot(modules []module) (map[string][32]byte, error) {
	result := make(map[string][32]byte)
	for _, mod := range modules {
		for _, name := range []string{"go.mod", "go.sum"} {
			path := filepath.Join(mod.root, name)
			data, err := os.ReadFile(path)
			if name == "go.sum" && errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			result[path] = sha256.Sum256(data)
		}
	}
	return result, nil
}

func logged(run runner, root string, env []string, path string, args ...string) (int, error) {
	log, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	code, err := run(root, env, log, args...)
	return code, errors.Join(err, log.Close())
}

func verify(mode, core, echo, nethttp, output string, run runner, progress io.Writer) (bool, error) {
	if err := os.MkdirAll(output, 0755); err != nil {
		return false, err
	}
	modules := []module{{"core", core}, {"echo", echo}, {"integration", filepath.Join(core, "test")}, {"openapi", filepath.Join(core, "test", "openapi")}}
	manifests := modules
	if mode == "platforms" {
		modules = []module{{"nethttp", nethttp}, {"platform-e2e", filepath.Join(core, "test", "http-platform")}}
		manifests = append([]module{{"core", core}, {"echo", echo}}, modules...)
	}
	before, err := snapshot(manifests)
	if err != nil {
		return false, err
	}
	tmp, err := os.MkdirTemp("", "spine-http-"+mode+"-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmp)
	results := make(map[string]any)
	success := true
	for _, mod := range modules {
		env := append(os.Environ(), "GOWORK=off")
		if mode == "platforms" {
			env = append(env, "GOTOOLCHAIN=go1.25.5")
		}
		prefix := []string{}
		if mod.name == "core" {
			prefix = []string{"run", "./scripts/test"}
		} else {
			for _, ext := range []string{"mod", "sum"} {
				data, err := os.ReadFile(filepath.Join(mod.root, "go."+ext))
				if ext == "sum" && errors.Is(err, os.ErrNotExist) {
					continue
				}
				if err != nil {
					return false, err
				}
				if err := os.WriteFile(filepath.Join(tmp, mod.name+"."+ext), data, 0600); err != nil {
					return false, err
				}
			}
			modfile := filepath.Join(tmp, mod.name+".mod")
			edits := []string{"mod", "edit", "-modfile=" + modfile, "-replace=github.com/NARUBROWN/spine=" + core, "-replace=github.com/NARUBROWN/spine-echo-platform=" + echo}
			if mode == "platforms" {
				edits = append(edits, "-replace=github.com/NARUBROWN/spine-nethttp-platform="+nethttp)
			}
			code, err := run(mod.root, env, progress, edits...)
			if err != nil {
				return false, err
			}
			if code != 0 {
				return false, fmt.Errorf("%s: go mod edit exited %d", mod.name, code)
			}
			flags := "-mod=mod -modfile=" + modfile
			if mode == "v3" {
				flags = strings.TrimSpace(os.Getenv("GOFLAGS") + " " + flags)
			}
			env = append(env, "GOFLAGS="+flags)
		}
		logPath := filepath.Join(output, mod.name+".jsonl")
		code, err := logged(run, mod.root, env, logPath, append(prefix, "test", "-json", "-race", "-count=1", "-timeout=120s", "./...")...)
		if err != nil {
			return false, err
		}
		log, err := os.Open(logPath)
		if err != nil {
			return false, err
		}
		counts, err := summarize(log)
		if err := errors.Join(err, log.Close()); err != nil {
			return false, err
		}
		counts["exit_code"] = code
		if code == 0 {
			vet, err := logged(run, mod.root, env, filepath.Join(output, mod.name+"-vet.log"), append(prefix, "vet", "./...")...)
			if err != nil {
				return false, err
			}
			counts["vet_exit_code"] = vet
		}
		success = success && code == 0 && counts["pass"] > 0 && counts["vet_exit_code"] == 0
		results[mod.name] = counts
		if err := json.NewEncoder(progress).Encode(map[string]any{mod.name: counts}); err != nil {
			return false, err
		}
	}
	after, err := snapshot(manifests)
	unchanged := err == nil && reflect.DeepEqual(before, after)
	results["manifests_unchanged"] = unchanged
	results["success"] = success && unchanged
	file, err := os.Create(filepath.Join(output, "summary.json"))
	if err != nil {
		return false, err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return success && unchanged, errors.Join(encoder.Encode(results), file.Close())
}

func run(args []string) error {
	if len(args) == 0 || (args[0] != "v3" && args[0] != "platforms") {
		return errors.New("usage: httpverify v3|platforms --echo-root DIR --output-dir DIR [--nethttp-root DIR]")
	}
	mode := args[0]
	flags := flag.NewFlagSet(mode, flag.ContinueOnError)
	echo := flags.String("echo-root", "", "Echo module root")
	nethttp := flags.String("nethttp-root", "", "NetHTTP module root")
	output := flags.String("output-dir", "", "Output directory")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *echo == "" || *output == "" || (mode == "platforms" && *nethttp == "") || flags.NArg() != 0 {
		return errors.New("missing or invalid verifier arguments")
	}
	for _, path := range []*string{echo, nethttp, output} {
		if *path == "" {
			continue
		}
		abs, err := filepath.Abs(*path)
		if err != nil {
			return err
		}
		*path = abs
	}
	core, err := os.Getwd()
	if err != nil {
		return err
	}
	success, err := verify(mode, core, *echo, *nethttp, *output, runGo, os.Stdout)
	if err != nil {
		return err
	}
	if !success {
		return errors.New("HTTP verification failed; see summary.json")
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
