package main

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestFinishRechecksCohortEvidence(t *testing.T) {
	source := strings.Repeat("a", 40)
	components := document{"core_version": "v0.7.5-preview.1", "modules": document{core + "-echo-platform": "v0.2.0"}}
	plan := document{"kind": "core", "version": "v0.7.5-preview.1", "source_sha": source, "components": components, "source_fingerprint": "original", "bootstrap_evidence": document{"digest": "sha256:original"}, "platform_evidence": document{core + "-echo-platform": document{"version": "v0.2.0", "source_sha": strings.Repeat("b", 40)}}}
	report := document{"result": "passed", "scope": "cohort", "version": plan["version"], "source_sha": source, "components": components,
		"selected_modules": document{core: plan["version"], core + "-echo-platform": "v0.2.0"}}
	repo := completionRepo{source: source}
	refreshed := 0
	refresh := func() (document, error) { refreshed++; return plan, nil }
	output, err := finish(repo, plan, report, "2", refresh)
	if err != nil || output["workflow_run_attempt"] != "2" || refreshed != 1 {
		t.Fatalf("output=%v err=%v refreshed=%d", output, err, refreshed)
	}
	stale := func() (document, error) {
		updated := make(document)
		for key, value := range plan {
			updated[key] = value
		}
		updated["source_fingerprint"] = "changed"
		return updated, nil
	}
	if output, err := finish(repo, plan, report, "2", stale); err == nil || output != nil {
		t.Fatalf("changed proof accepted: %v %v", output, err)
	}
	changedPeer := func() (document, error) {
		updated := maps.Clone(plan)
		updated["platform_evidence"] = document{core + "-echo-platform": document{"version": "v0.2.0", "source_sha": strings.Repeat("f", 40)}}
		return updated, nil
	}
	if output, err := finish(repo, plan, report, "2", changedPeer); err == nil || output != nil {
		t.Fatalf("changed platform evidence accepted: %v %v", output, err)
	}
	report["scope"] = "core"
	if output, err := finish(repo, plan, report, "2", refresh); err == nil || output != nil || refreshed != 1 {
		t.Fatalf("wrong scope accepted: %v %v", output, err)
	}
}

func TestRunFinishWritesManifestAndPublishesOnlyStable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("GitHub command fixture uses a POSIX shell")
	}
	root := t.TempDir()
	gh := filepath.Join(root, "gh")
	if err := os.WriteFile(gh, []byte(`#!/bin/sh
case "$1 $2" in
  "api --method")
    printf '{"object":{"type":"commit","sha":"%s"}}\n' "$EXPECTED_SHA"
    ;;
  "release create")
    printf '%s\n' "$@" > "$GH_CALL"
    ;;
  *) exit 1 ;;
esac
`), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GITHUB_REPOSITORY", "NARUBROWN/spine-echo-platform")
	t.Setenv("GITHUB_RUN_ATTEMPT", "2")
	source := strings.Repeat("a", 40)
	t.Setenv("EXPECTED_SHA", source)

	for _, tc := range []struct{ channel, version, result string }{
		{"preview", "v0.2.0-preview.1", "passed"},
		{"rc", "v0.2.0-rc.1", "passed"},
		{"stable", "v0.2.0", "passed"},
		{"stable", "v0.2.0", "failed"},
	} {
		t.Run(tc.channel+"/"+tc.result, func(t *testing.T) {
			dir := t.TempDir()
			planFile, reportFile := filepath.Join(dir, "plan.json"), filepath.Join(dir, "report.json")
			manifestFile, callFile := filepath.Join(dir, "release-manifest.json"), filepath.Join(dir, "gh-call")
			t.Setenv("GH_CALL", callFile)
			module := core + "-echo-platform"
			components := document{"core_version": "v0.7.5"}
			plan := document{"kind": "platform", "module": module, "channel": tc.channel, "version": tc.version, "source_sha": source, "components": components, "workflow_run_attempt": "1"}
			report := document{"result": tc.result, "scope": "platform", "version": tc.version, "source_sha": source, "components": components, "selected_modules": document{core: "v0.7.5", module: tc.version}}
			if err := writePlan(planFile, plan); err != nil {
				t.Fatal(err)
			}
			if err := writePlan(reportFile, report); err != nil {
				t.Fatal(err)
			}
			err := runFinish(planFile, reportFile, manifestFile)
			if tc.result == "failed" {
				if err == nil {
					t.Fatal("failed report accepted")
				}
				if _, err := os.Stat(manifestFile); !os.IsNotExist(err) {
					t.Fatalf("manifest stat: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(manifestFile)
				if err != nil {
					t.Fatal(err)
				}
				manifest, err := decodeDocument(data)
				plan["workflow_run_attempt"] = "2"
				plan["validation"] = report
				if err != nil || !reflect.DeepEqual(manifest, plan) {
					t.Fatalf("manifest %v, error %v", manifest, err)
				}
			}
			call, err := os.ReadFile(callFile)
			if tc.channel != "stable" || tc.result != "passed" {
				if !os.IsNotExist(err) {
					t.Fatalf("unexpected release creation: %s, %v", call, err)
				}
				return
			}
			if err != nil || !strings.HasPrefix(string(call), "release\ncreate\n"+tc.version+"\n"+manifestFile+"\n") ||
				!strings.Contains(string(call), "\n--verify-tag\n") || !strings.Contains(string(call), "\n--latest\n") ||
				strings.Contains(string(call), "--prerelease") {
				t.Fatalf("release command %s, error %v", call, err)
			}
		})
	}
}
