package main

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestRecordCoreCandidateRequiresPublishedTagEvidence(t *testing.T) {
	const version = "v0.7.5-preview.1"
	source := strings.Repeat("a", 40)
	raw := func(value string) json.RawMessage {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	components := json.RawMessage(`{"core_version":"v0.7.5-preview.1","modules":{}}`)
	plan := document{"kind": raw("core"), "source_sha": raw(source), "version": raw(version),
		"components": components, "workflow_run_attempt": raw("1")}
	report := document{"scope": raw("core"), "result": raw("passed"), "source_sha": raw(source),
		"version": raw(version), "components": components,
		"selected_modules": json.RawMessage(`{"github.com/NARUBROWN/spine":"v0.7.5-preview.1"}`)}
	for _, scenario := range []string{"valid", "wrong scope", "failed check", "wrong source", "wrong components", "wrong target", "missing core", "wrong core", "source pseudo-version", "moved tag", "lookup failure"} {
		t.Run(scenario, func(t *testing.T) {
			input := maps.Clone(report)
			mutate := func(key, value string) { input[key] = raw(value) }
			switch scenario {
			case "wrong scope":
				mutate("scope", "cohort")
			case "failed check":
				mutate("result", "failed")
			case "wrong source":
				mutate("source_sha", strings.Repeat("b", 40))
			case "wrong components":
				input["components"] = json.RawMessage(`{}`)
			case "wrong target":
				mutate("version", source)
			case "missing core":
				delete(input, "selected_modules")
			case "wrong core":
				input["selected_modules"] = json.RawMessage(`{"github.com/NARUBROWN/spine":"v0.7.4"}`)
			case "source pseudo-version":
				input["selected_modules"] = json.RawMessage(`{"github.com/NARUBROWN/spine":"v0.7.5-0.20261114000000-aaaaaaaaaaaa"}`)
			}
			manifest, err := record(plan, input, "2", func(tag string) (string, error) {
				if tag != version {
					t.Fatalf("tag = %q", tag)
				}
				switch scenario {
				case "moved tag":
					return strings.Repeat("b", 40), nil
				case "lookup failure":
					return "", errors.New("request failed")
				}
				return source, nil
			})
			if scenario != "valid" {
				if err == nil || manifest != nil {
					t.Fatalf("record = %v, %v", manifest, err)
				}
				return
			}
			var validation document
			if err != nil || field(manifest, "workflow_run_attempt") != "2" ||
				json.Unmarshal(manifest["validation"], &validation) != nil || !reflect.DeepEqual(validation, input) ||
				field(plan, "workflow_run_attempt") != "1" {
				t.Fatalf("manifest = %v, %v", manifest, err)
			}
		})
	}
}

func TestRecordCommandKeepsWorkflowRunAndValidation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("GitHub command fixture uses a POSIX shell")
	}
	root := t.TempDir()
	const version = "v0.7.5-preview.1"
	source := strings.Repeat("a", 40)
	gh := filepath.Join(root, "gh")
	if err := os.WriteFile(gh, []byte(`#!/bin/sh
[ "$*" = "api --method GET repos/NARUBROWN/spine/git/ref/tags/v0.7.5-preview.1" ] || exit 1
printf '{"object":{"type":"commit","sha":"%s"}}\n' "$EXPECTED_SHA"
`), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GITHUB_REPOSITORY", "NARUBROWN/spine")
	t.Setenv("GITHUB_RUN_ATTEMPT", "3")
	t.Setenv("GITHUB_RUN_ID", "999")
	t.Setenv("EXPECTED_SHA", source)
	components := map[string]string{"core_version": version}
	planPath, reportPath, manifestPath := filepath.Join(root, "plan.json"), filepath.Join(root, "report.json"), filepath.Join(root, "manifest.json")
	plan := map[string]any{"kind": "core", "version": version, "source_sha": source, "components": components, "workflow_run_id": "42", "workflow_run_attempt": "1"}
	report := map[string]any{"scope": "core", "result": "passed", "version": version, "source_sha": source, "components": components, "selected_modules": map[string]string{"github.com/NARUBROWN/spine": version}}
	for path, value := range map[string]map[string]any{planPath: plan, reportPath: report} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := run([]string{"--plan", planPath, "--report", reportPath, "--manifest", manifestPath}); err != nil {
		t.Fatal(err)
	}
	manifest, err := read(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	plan["workflow_run_attempt"] = "3"
	plan["validation"] = report
	expected, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := json.Marshal(manifest)
	if err != nil || !equal(actual, expected) {
		t.Fatalf("manifest %s, want %s, error %v", actual, expected, err)
	}
}
