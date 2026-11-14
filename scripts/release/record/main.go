package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"

	"github.com/NARUBROWN/spine/scripts/release/internal/releasegithub"
)

type document map[string]json.RawMessage

func read(path string) (document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result document
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("expected a JSON object")
	}
	return result, nil
}

func field(doc document, key string) string {
	var value string
	_ = json.Unmarshal(doc[key], &value)
	return value
}

func equal(left, right json.RawMessage) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && reflect.DeepEqual(a, b)
}

func record(plan, report document, attempt string, tagSHA func(string) (string, error)) (document, error) {
	version, source := field(plan, "version"), field(plan, "source_sha")
	mismatch := errors.New("Public verification results for the core candidate do not match the plan")
	if field(plan, "kind") != "core" || field(report, "scope") != "core" || field(report, "result") != "passed" ||
		version == "" || source == "" || field(report, "version") != version || field(report, "source_sha") != source ||
		!equal(plan["components"], report["components"]) {
		return nil, mismatch
	}
	tagged, err := tagSHA(version)
	if err != nil {
		return nil, err
	}
	if tagged != source {
		return nil, mismatch
	}
	var components struct {
		Core string `json:"core_version"`
	}
	var selected map[string]string
	if json.Unmarshal(plan["components"], &components) != nil ||
		json.Unmarshal(report["selected_modules"], &selected) != nil ||
		components.Core == "" || selected["github.com/NARUBROWN/spine"] != components.Core {
		return nil, errors.New("The public consumer did not select the planned core version")
	}
	manifest := make(document, len(plan)+2)
	for name, value := range plan {
		manifest[name] = value
	}
	manifest["workflow_run_attempt"], err = json.Marshal(attempt)
	if err != nil {
		return nil, err
	}
	manifest["validation"], err = json.Marshal(report)
	return manifest, err
}

func run(args []string) error {
	flags := flag.NewFlagSet("record-core", flag.ContinueOnError)
	planPath := flags.String("plan", "release-plan.json", "release plan JSON")
	reportPath := flags.String("report", "release-validation.json", "public validation JSON")
	manifestPath := flags.String("manifest", "release-manifest.json", "output manifest path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	plan, err := read(*planPath)
	if err != nil {
		return err
	}
	report, err := read(*reportPath)
	if err != nil {
		return err
	}
	client, err := releasegithub.New(os.Getenv("GITHUB_REPOSITORY"), "")
	if err != nil {
		return err
	}
	attempt, ok := os.LookupEnv("GITHUB_RUN_ATTEMPT")
	if !ok {
		attempt = "1"
	}
	manifest, err := record(plan, report, attempt, client.TagSHA)
	if err != nil {
		return err
	}
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return err
	}
	return os.WriteFile(*manifestPath, data.Bytes(), 0644)
}

func main() {
	if err := run(os.Args[1:]); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "Release aborted:", err)
		os.Exit(1)
	}
}
