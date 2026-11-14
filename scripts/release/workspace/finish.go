package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/NARUBROWN/spine/scripts/release/internal/releasegithub"
)

func finish(repo releaseRepository, plan, report document, attempt string, refresh func() (document, error)) (document, error) {
	version, source := textField(plan, "version"), textField(plan, "source_sha")
	scope := "platform"
	if textField(plan, "kind") == "core" {
		scope = "cohort"
	}
	mismatch := errors.New("Public module verification results do not match the publication plan")
	if version == "" || source == "" || textField(report, "source_sha") != source || textField(report, "version") != version ||
		textField(report, "result") != "passed" || !reflect.DeepEqual(report["components"], plan["components"]) ||
		textField(report, "scope") != scope || !selectedModulesMatch(plan, report, scope) {
		return nil, mismatch
	}
	tagged, err := repo.TagSHA(version)
	if err != nil {
		return nil, err
	}
	if tagged != source {
		return nil, mismatch
	}
	if scope == "cohort" {
		fresh, err := refresh()
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(fresh)
		if err != nil {
			return nil, err
		}
		fresh, err = decodeDocument(data)
		if err != nil {
			return nil, err
		}
		for _, key := range []string{"source_sha", "source_fingerprint", "components", "bootstrap_evidence", "platform_evidence"} {
			if !reflect.DeepEqual(plan[key], fresh[key]) {
				return nil, errors.New("Candidate evidence changed during component verification")
			}
		}
	}
	manifest := make(document, len(plan)+2)
	for key, value := range plan {
		manifest[key] = value
	}
	manifest["workflow_run_attempt"] = attempt
	manifest["validation"] = report
	return manifest, nil
}

func peerTag(module, version string) (string, error) {
	client, err := releasegithub.New(strings.TrimPrefix(module, "github.com/"), "")
	if err != nil {
		return "", err
	}
	return client.TagSHA(version)
}

func runFinish(planPath, reportPath, manifestPath string) error {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return err
	}
	plan, err := decodeDocument(data)
	if err != nil {
		return err
	}
	data, err = os.ReadFile(reportPath)
	if err != nil {
		return err
	}
	report, err := decodeDocument(data)
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
	version := textField(plan, "version")
	manifest, err := finish(repo, plan, report, attempt, func() (document, error) {
		tagged, err := policyAt(repo, textField(plan, "source_sha"))
		if err != nil {
			return nil, err
		}
		p, err := decodePolicy(tagged)
		if err != nil {
			return nil, err
		}
		return completePlan(repo, repository, p, tagged, version, os.Getenv("GITHUB_REF"), os.Getenv("GITHUB_RUN_ID"), attempt, peerTag)
	})
	if err != nil {
		return err
	}
	path, err := filepath.Abs(manifestPath)
	if err != nil {
		return err
	}
	if err := writePlan(path, manifest); err != nil {
		return err
	}
	if textField(plan, "channel") != "stable" {
		return nil
	}
	return publishRelease(repository, plan, path)
}

func publishRelease(repository string, plan document, manifest string) error {
	version := textField(plan, "version")
	match := tag.FindStringSubmatch(version)
	if match == nil {
		return errors.New("invalid stable release version")
	}
	directory, err := os.MkdirTemp("", "spine-release-publish-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	notes := filepath.Join(directory, "notes.md")
	source := textField(plan, "source_sha")
	stage := textField(plan, "source_tag")
	if stage == "" {
		stage = "preview"
	}
	reason := textField(plan, "observation_override_reason")
	if reason == "" {
		reason = "none"
	}
	content := fmt.Sprintf("Commit: `%s`\n\nChannel branch: `%s`\n\nPrevious stage: `%s`\n\nMigration guide: https://github.com/%s/blob/%s/docs/migration/%s.md\n\nObservation period override reason: %s\n\nSee the attached manifest for verified core and platform versions and execution evidence. Examples are updated after the stable release.\n", source, textField(plan, "branch"), stage, repository, source, match[1], reason)
	if err := os.WriteFile(notes, []byte(content), 0600); err != nil {
		return err
	}
	return execute("", "gh", "release", "create", version, manifest, "--repo", repository, "--verify-tag", "--title", version, "--notes-file", notes, "--latest")
}
