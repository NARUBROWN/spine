package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/NARUBROWN/spine/scripts/release/internal/releasegithub"
)

const core = "github.com/NARUBROWN/spine"

var platforms = []string{"echo-platform", "gorilla-platform", "kafka-platform", "nats-platform", "nethttp-platform", "rabbitmq-platform", "validator-playground"}
var tag = regexp.MustCompile(`^(v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))(?:-(preview|rc)\.[1-9][0-9]*)?$`)
var sha = regexp.MustCompile(`^[a-f0-9]{40}$`)
var channels = []string{"preview", "rc", "stable"}

type selection struct {
	Core    string            `json:"core_version"`
	Modules map[string]string `json:"modules"`
}

type policy struct {
	Schema       string               `json:"schema_version"`
	Next         string               `json:"next_version"`
	PreviewHours int                  `json:"preview_hours"`
	RCHours      int                  `json:"rc_hours"`
	StableHours  int                  `json:"stable_interval_hours"`
	Kind         string               `json:"kind"`
	Module       string               `json:"module"`
	Channels     map[string]selection `json:"channels"`
	Environment  string               `json:"approval_environment"`
	Reviewer     string               `json:"reviewer_login"`
	Branches     map[string]string    `json:"branches"`
	Sources      map[string]string    `json:"ci_sources"`
	CICore       string               `json:"ci_core"`
}

func channelFor(ref string) string {
	switch ref {
	case "main":
		return "stable"
	case "preview", "rc":
		return ref
	}
	if match := tag.FindStringSubmatch(ref); match != nil {
		if match[2] != "" {
			return match[2]
		}
		return "stable"
	}
	return "preview"
}

func knownPlatform(module string) bool {
	for _, suffix := range platforms {
		if module == core+"-"+suffix {
			return true
		}
	}
	return false
}

func (p policy) validate() error {
	if p.Schema != "spine.release-policy/v2" {
		return errors.New("Unsupported release policy")
	}
	if !tag.MatchString(p.Next) || channelFor(p.Next) != "stable" {
		return errors.New("next_version must specify a stable version")
	}
	for _, hours := range []struct {
		name  string
		value int
	}{
		{"preview_hours", p.PreviewHours}, {"rc_hours", p.RCHours}, {"stable_interval_hours", p.StableHours},
	} {
		if hours.value <= 0 {
			return fmt.Errorf("%s must specify a positive number of hours", hours.name)
		}
	}
	if (p.Kind != "core" && p.Kind != "platform") || (p.Module != core && !knownPlatform(p.Module)) {
		return errors.New("Specify the module path of the core or an official platform")
	}
	if (p.Kind == "core") != (p.Module == core) {
		return errors.New("Release kind does not match the module path")
	}
	if len(p.Channels) != 3 {
		return errors.New("Specify component versions for each channel")
	}
	var base string
	for _, channel := range channels {
		selected, exists := p.Channels[channel]
		if !exists {
			return errors.New("Specify component versions for each channel")
		}
		match := tag.FindStringSubmatch(selected.Core)
		if match == nil || channelFor(selected.Core) != channel || (base != "" && match[1] != base) {
			return errors.New("Platforms must reference the same target core version and channel")
		}
		base = match[1]
		if p.Kind == "core" && (base != p.Next || len(selected.Modules) != len(platforms)) {
			return errors.New("The core must pin all official platform versions for each channel")
		}
		if p.Kind == "platform" && len(selected.Modules) != 0 {
			return errors.New("Platform releases specify only the core dependency")
		}
		for module, version := range selected.Modules {
			if !knownPlatform(module) {
				return errors.New("Integration verification requires official platform modules")
			}
			if !tag.MatchString(version) {
				return errors.New("Version must use the vX.Y.Z-preview.N, vX.Y.Z-rc.N or vX.Y.Z format")
			}
		}
	}
	if p.Environment != "release-stable" || p.Reviewer == "" {
		return errors.New("A stable release approval environment and designated reviewers are required")
	}
	if len(p.Branches) != 3 || p.Branches["preview"] != "preview" || p.Branches["rc"] != "rc" || p.Branches["stable"] != "main" {
		return errors.New("Release branches must follow preview → rc → main")
	}
	for module, ref := range p.Sources {
		if !knownPlatform(module) || !sha.MatchString(ref) {
			return errors.New("Source CI requires full commit SHAs for official platforms")
		}
	}
	if p.Kind == "core" && len(p.Sources) != len(platforms) {
		return errors.New("Core source CI requires commits for all official platforms")
	}
	if p.Kind == "platform" && !sha.MatchString(p.CICore) {
		return errors.New("Platform source CI requires the full core commit SHA")
	}
	return nil
}

func appendFile(path string, write func(io.Writer) error) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	return errors.Join(write(file), file.Close())
}

func platformRoots(root string) []string {
	roots := []string{root}
	for _, name := range platforms {
		roots = append(roots, filepath.Join(filepath.Dir(root), "spine-"+name))
	}
	return roots
}

func prepare(root, output string, p policy, published bool) error {
	members := []string{root, filepath.Join(root, "spine-core")}
	dirs := map[string]string{core: members[1]}
	if p.Kind == "core" {
		members = platformRoots(root)
		dirs[core] = root
		for i, name := range platforms {
			dirs[core+"-"+name] = members[i+1]
		}
		for _, name := range childModules {
			members = append(members, filepath.Join(root, name))
		}
	}
	var content strings.Builder
	content.WriteString("go 1.26.0\n\nuse (\n")
	for _, member := range members {
		info, err := os.Stat(filepath.Join(member, "go.mod"))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("Source CI module not found: %s", member)
		}
		canonical, err := filepath.EvalSymlinks(member)
		if err != nil {
			return err
		}
		fmt.Fprintf(&content, "\t%q\n", canonical)
	}
	content.WriteString(")\n")
	if !published {
		content.WriteString("\nreplace (\n")
		for _, channel := range channels {
			selected := p.Channels[channel]
			replacements := []string{core}
			for _, platform := range platforms {
				module := core + "-" + platform
				if _, ok := selected.Modules[module]; ok {
					replacements = append(replacements, module)
				}
			}
			for _, module := range replacements {
				version := selected.Modules[module]
				if module == core {
					version = selected.Core
				}
				dir, err := filepath.EvalSymlinks(dirs[module])
				if err != nil {
					return err
				}
				fmt.Fprintf(&content, "\t%s %s => %q\n", module, version, dir)
			}
		}
		content.WriteString(")\n")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(output, []byte(content.String()), 0666); err != nil {
		return err
	}
	if env := os.Getenv("GITHUB_ENV"); env != "" {
		return appendFile(env, func(w io.Writer) error { _, err := fmt.Fprintln(w, "GOWORK="+output); return err })
	}
	return nil
}

func check(root string, p policy) error {
	roots := []string{root}
	if p.Kind == "core" {
		roots = platformRoots(root)
	}
	for _, member := range roots {
		cmd := exec.Command("go", "mod", "edit", "-json")
		cmd.Dir, cmd.Stderr = member, os.Stderr
		data, err := cmd.Output()
		if err != nil {
			return err
		}
		var mod struct{ Replace []json.RawMessage }
		if err := json.Unmarshal(data, &mod); err != nil {
			return err
		}
		if len(mod.Replace) > 0 {
			return errors.New("Do not commit replace directives in platform or core modules, including for source CI")
		}
		if err := execute(member, "go", "mod", "verify"); err != nil {
			return err
		}
	}
	return execute(root, "git", "diff", "--exit-code", "--", "go.mod", "go.sum", ":(glob)**/go.mod", ":(glob)**/go.sum")
}

func execute(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, os.Stdout, os.Stderr
	return cmd.Run()
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: workspace refs|prepare|check|channel [options]")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	channel := flags.String("channel", "", "publication channel")
	apply := flags.Bool("apply", false, "apply module pins")
	tidy := flags.Bool("tidy", false, "resolve and verify modules after applying pins")
	version := flags.String("version", "", "core candidate version")
	reportPath := flags.String("report", "release-validation.json", "validation report path")
	manifestPath := flags.String("manifest", "release-manifest.json", "manifest output path")
	planPath := flags.String("plan", "release-plan.json", "release plan path")
	root := flags.String("root", ".", "repository root")
	output := flags.String("output", "", "workspace path")
	published := flags.Bool("published", false, "use published dependencies")
	ref := flags.String("ref", "preview", "channel or tag")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	if args[0] == "channel" {
		fmt.Println(channelFor(*ref))
		return nil
	}
	abs, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(canonical, ".github", "release-policy.json"))
	if err != nil {
		return err
	}
	if args[0] == "post-examples" {
		raw, err := decodeDocument(data)
		if err != nil {
			return err
		}
		return runExamples(raw, *version)
	}

	var p policy
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	if args[0] == "check" {
		return check(canonical, p)
	}
	if err := p.validate(); err != nil {
		return err
	}
	switch args[0] {
	case "pin", "check-module":
		if _, ok := p.Channels[*channel]; !ok {
			return errors.New("--channel must be preview, rc or stable")
		}
		if args[0] == "check-module" {
			return checkModule(canonical, p, *channel)
		}
		if *tidy && !*apply {
			return errors.New("--tidy requires --apply")
		}
		return pin(canonical, p, *channel, *apply, *tidy, os.Stdout)
	case "finish":
		return runFinish(*planPath, *reportPath, *manifestPath)
	case "plan", "tag":
		return runPublication(args[0], canonical, *planPath, p, data)
	case "complete-plan":
		raw, err := decodeDocument(data)
		if err != nil {
			return err
		}
		repository := os.Getenv("GITHUB_REPOSITORY")
		repo, err := releasegithub.New(repository, "")
		if err != nil {
			return err
		}
		attempt, ok := os.LookupEnv("GITHUB_RUN_ATTEMPT")
		if !ok {
			attempt = "1"
		}
		candidate, err := completePlan(repo, repository, p, raw, *version, os.Getenv("GITHUB_REF"), os.Getenv("GITHUB_RUN_ID"), attempt, func(module, version string) (string, error) {
			client, err := releasegithub.New(strings.TrimPrefix(module, "github.com/"), "")
			if err != nil {
				return "", err
			}
			return client.TagSHA(version)
		})
		if err != nil {
			return err
		}
		result, err := json.MarshalIndent(candidate, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*planPath, append(result, '\n'), 0644); err != nil {
			return err
		}
		if output := os.Getenv("GITHUB_OUTPUT"); output != "" {
			return appendFile(output, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "sha=%s\nversion=%s\nchannel=%s\n", textField(candidate, "source_sha"), textField(candidate, "version"), textField(candidate, "channel"))
				return err
			})
		}
		return nil
	case "validate-policy":
		return checkPublicationDocs(canonical, p)
	case "prepare":
		if *output == "" {
			return errors.New("prepare requires --output")
		}
		target, err := filepath.Abs(*output)
		if err != nil {
			return err
		}
		return prepare(canonical, target, p, *published)
	case "refs":
		selected := p.Sources
		if *published {
			selected = p.Channels[channelFor(*ref)].Modules
		}
		return appendFile(os.Getenv("GITHUB_OUTPUT"), func(w io.Writer) error {
			for _, suffix := range platforms {
				name := strings.TrimSuffix(suffix, "-platform")
				if name == "validator-playground" {
					name = "playground"
				}
				if value, ok := selected[core+"-"+suffix]; ok {
					if _, err := fmt.Fprintf(w, "%s=%s\n", name, value); err != nil {
						return err
					}
				}
			}
			return nil
		})
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil && !errors.Is(err, flag.ErrHelp) {
		if len(os.Args) > 1 && (os.Args[1] == "plan" || os.Args[1] == "tag" || os.Args[1] == "finish") {
			fmt.Fprintln(os.Stderr, "Release aborted:", err)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
