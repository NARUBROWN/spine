package main

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/NARUBROWN/spine/scripts/release/internal/publiccheck"
	"github.com/NARUBROWN/spine/scripts/release/internal/releasegithub"
)

var exampleRepository = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type exampleRunner interface {
	ReadModule(string) (publiccheck.ModuleEdit, error)
	Execute(string, ...string) error
	Modules(string, map[string]string) (map[string]string, error)
	Tests(string, ...string) error
}

func checkExample(dir, version string, expected map[string]string, r exampleRunner) error {
	parsed, err := r.ReadModule(dir)
	if err != nil {
		return err
	}
	declared := make(map[string]string)
	for _, req := range parsed.Require {
		declared[req.Path] = req.Version
	}
	mismatch := fmt.Errorf("Publish a commit updating the example go.mod to the stable component versions first: %s", dir)
	if declared[core] != version {
		return mismatch
	}
	selected := make(map[string]string)
	for module, want := range expected {
		if current, ok := declared[module]; ok {
			if current != want {
				return mismatch
			}
			selected[module] = want
		}
	}
	for _, step := range []string{"download", "verify"} {
		if err := r.Execute(dir, "go", "mod", step); err != nil {
			return err
		}
	}
	if _, err := r.Modules(dir, selected); err != nil {
		return err
	}
	return r.Tests(dir, "-race", "./...", "-count=1")
}

func verifyExamples(version, ref, work string, raw document, env map[string]string, r, msaRunner exampleRunner,
	proof func() (document, error), checkout func(string, string, string) error) (document, error) {
	if !tag.MatchString(version) || channelFor(version) != "stable" || ref != "refs/heads/main" {
		return nil, errors.New("Public example verification requires a stable tag and the main branch")
	}
	if env["SPINE_TEST_KAFKA_BROKER"] == "" || env["SPINE_TEST_RABBITMQ_URL"] == "" {
		return nil, errors.New("Example verification requires both brokers")
	}
	manifest, err := proof()
	if err != nil {
		return nil, err
	}
	components := objectField(manifest, "components")
	expected := map[string]string{core: version}
	for key, value := range objectField(components, "modules") {
		pin, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("invalid component version for %s", key)
		}
		expected[key] = pin
	}
	specs := objectField(raw, "post_release_examples")
	if len(specs) != 2 || objectField(specs, "chat") == nil || objectField(specs, "msa") == nil {
		return nil, errors.New("Specify chat and MSA example commits to verify after the stable release")
	}
	for _, name := range []string{"chat", "msa"} {
		spec := objectField(specs, name)
		if !exampleRepository.MatchString(textField(spec, "repository")) || !sha.MatchString(textField(spec, "ref")) {
			return nil, errors.New("Pin the example repository and full commit SHA")
		}
	}
	for _, name := range []string{"chat", "msa"} {
		spec := objectField(specs, name)
		if err := checkout(filepath.Join(work, name), textField(spec, "repository"), textField(spec, "ref")); err != nil {
			return nil, err
		}
	}
	chat, msa := filepath.Join(work, "chat"), filepath.Join(work, "msa")
	for _, dir := range []string{chat, filepath.Join(msa, "order-app"), filepath.Join(msa, "stock-app")} {
		if err := checkExample(dir, version, expected, r); err != nil {
			return nil, err
		}
	}
	if err := r.Tests(chat, "-race", "-tags=e2e", "./e2e", "-count=1", "-timeout=3m"); err != nil {
		return nil, err
	}
	if err := msaRunner.Tests(msa, "-race", "-tags=e2e", "./e2e", "-count=1", "-timeout=4m"); err != nil {
		return nil, err
	}
	return document{"result": "passed", "version": version, "examples": specs, "components": components}, nil
}

func checkoutExample(destination, repository, ref string) error {
	for _, args := range [][]string{
		{"clone", "--no-checkout", "--filter=blob:none", "https://github.com/" + repository + ".git", destination},
		{"-C", destination, "checkout", "--detach", ref},
	} {
		if data, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %s: %w", args[0], strings.TrimSpace(string(data)), err)
		}
	}
	data, err := exec.Command("git", "-C", destination, "rev-parse", "HEAD").Output()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(data)) != ref {
		return errors.New("The example commit does not match the manifest")
	}
	return nil
}

func runExamples(raw document, version string) error {
	work, err := os.MkdirTemp("", "spine-post-release-examples-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	env := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	env["GOWORK"] = "off"
	env["GOMODCACHE"] = filepath.Join(work, "modules")
	env["GOFLAGS"] = "-modcacherw"
	msaEnv := maps.Clone(env)
	msaEnv["GOWORK"] = filepath.Join(work, "msa", "go.work")
	msaEnv["SPINE_DEMO_E2E_BROKER"] = env["SPINE_TEST_KAFKA_BROKER"]
	repository := os.Getenv("GITHUB_REPOSITORY")
	verify := func() (document, error) {
		repo, err := releasegithub.New(repository, "")
		if err != nil {
			return nil, err
		}
		source, err := repo.TagSHA(version)
		if err != nil {
			return nil, err
		}
		tagged, err := policyAt(repo, source)
		if err != nil {
			return nil, err
		}
		p, err := decodePolicy(tagged)
		if err != nil {
			return nil, err
		}
		_, manifest, err := verifyCandidate(repo, repository, version, "stable", p, false)
		return manifest, err
	}
	report, err := verifyExamples(version, os.Getenv("GITHUB_REF"), work, raw, env,
		publiccheck.NewRunner(env, os.Stdout), publiccheck.NewRunner(msaEnv, os.Stdout), verify, checkoutExample)
	if err != nil {
		return err
	}
	return writePlan("post-release-examples.json", report)
}
