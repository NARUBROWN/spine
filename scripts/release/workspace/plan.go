package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/NARUBROWN/spine/scripts/release/internal/sourceproof"
)

func newer(left, right string) bool {
	l, r := strings.Split(left, "."), strings.Split(right, ".")
	for i, value := range l {
		if value == r[i] {
			continue
		}
		if len(value) != len(r[i]) {
			return len(value) > len(r[i])
		}
		return value > r[i]
	}
	return false
}

func requireAge(value string, hours int, now time.Time, reason string) error {
	published, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return errors.New("Invalid publication or verification timestamp")
	}
	age := now.Sub(published).Hours()
	if age < 0 {
		return errors.New("A release with a publication time in the future cannot be used")
	}
	if age < float64(hours) && reason == "" {
		return fmt.Errorf("Insufficient observation period: %.1f hours elapsed, %d hours required", age, hours)
	}
	return nil
}

type planningRepo interface {
	releaseRepository
	Releases() ([]json.RawMessage, error)
}

func makePlan(repo planningRepo, repository string, p policy, raw document, channel, version, source, reason, runID, attempt string, now time.Time, approval func() error) (document, error) {
	if repository != strings.TrimPrefix(p.Module, "github.com/") {
		return nil, errors.New("The module specified by the policy does not match the current repository")
	}
	match := tag.FindStringSubmatch(version)
	if match == nil || match[1] != p.Next || channelFor(version) != channel {
		return nil, errors.New("Target version and channel do not match the policy next_version")
	}
	reason = strings.TrimSpace(reason)
	if reason != "" && utf8.RuneCountInString(reason) < 20 {
		return nil, errors.New("Provide an emergency observation period override reason of at least 20 characters")
	}
	tagged, err := repo.TagSHA(version)
	if err != nil {
		return nil, err
	}
	if tagged != "" {
		return nil, errors.New("Existing tags must not be moved or reused")
	}
	if channel != "stable" {
		tagged, err := repo.TagSHA(match[1])
		if err != nil {
			return nil, err
		}
		if tagged != "" {
			return nil, errors.New("Prereleases cannot be added to an already published stable version")
		}
		var refs []struct{ Ref string }
		if err := fetch(repo, "git/matching-refs/tags/"+match[1]+"-"+channel+".", &refs); err != nil {
			return nil, err
		}
		sequence := strings.Split(version, ".")[3]
		for _, ref := range refs {
			name := strings.TrimPrefix(ref.Ref, "refs/tags/")
			if tag.MatchString(name) && channelFor(name) != "stable" && !newer(sequence, strings.Split(name, ".")[3]) {
				return nil, errors.New("The new channel sequence number must exceed the existing number")
			}
		}
	}
	branch := p.Branches[channel]
	var head struct{ Object struct{ SHA string } }
	if err := fetch(repo, "git/ref/heads/"+branch, &head); err != nil {
		return nil, err
	}
	target := head.Object.SHA
	if !sha.MatchString(target) {
		return nil, errors.New("Channel branch does not point to a valid commit")
	}
	sourceTag, upstream := "", ""
	var pr, evidence any
	if channel == "preview" {
		if target != source {
			return nil, errors.New("Preview source requires the full commit SHA of the preview branch HEAD")
		}
	} else {
		sourceTag = source
		prior := tag.FindStringSubmatch(source)
		if prior == nil || prior[1] != match[1] {
			return nil, errors.New("A candidate for a different version cannot be promoted")
		}
		expected, hours := "preview", p.PreviewHours
		if channel == "stable" {
			expected, hours = "rc", p.RCHours
		}
		artifact, manifest, err := verifyCandidate(repo, repository, source, expected, p, false)
		if err != nil {
			return nil, err
		}
		evidence = document{"artifact_id": artifact.ID, "digest": artifact.Digest, "created_at": artifact.CreatedAt, "workflow_run_id": manifest["workflow_run_id"]}
		upstream = textField(manifest, "source_sha")
		pr, err = promotion(repo, repository, target, upstream, branch, p.Branches[expected], version)
		if err != nil {
			return nil, err
		}
		previous := objectField(manifest, "components")
		priorCore := tag.FindStringSubmatch(textField(previous, "core_version"))
		nextCore := tag.FindStringSubmatch(p.Channels[channel].Core)
		prevModules := objectField(previous, "modules")
		nextModules := objectField(channelComponents(raw, channel), "modules")
		if priorCore == nil || nextCore == nil || priorCore[1] != nextCore[1] ||
			!(len(prevModules) == 0 && len(nextModules) == 0 || reflect.DeepEqual(prevModules, nextModules)) {
			return nil, errors.New("Platform tags cannot change during core promotion. Publish a new Preview.")
		}
		if err := requireAge(artifact.CreatedAt, hours, now, reason); err != nil {
			return nil, err
		}
	}
	tree, err := sourceproof.Tree(repo, target)
	if err != nil {
		return nil, err
	}
	checked, err := policyAt(repo, target)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(checked, raw) {
		return nil, errors.New("The candidate commit release policy differs from the current plan. Check the target SHA.")
	}
	if channel == "stable" {
		if err := approval(); err != nil {
			return nil, err
		}
		if err := checkStableReleases(repo, match[1], p.StableHours, now, reason); err != nil {
			return nil, err
		}
	}
	fingerprint, err := sourceproof.Fingerprint(repo, target)
	if err != nil {
		return nil, err
	}
	return document{
		"schema_version": "spine.release-manifest/v1", "channel": channel, "version": version, "source_sha": target,
		"source_tag": sourceTag, "branch": branch, "source_tree": tree, "source_fingerprint": fingerprint, "kind": p.Kind,
		"module": p.Module, "upstream_source_sha": upstream, "promotion_pr": pr, "components": channelComponents(raw, channel),
		"source_evidence": evidence, "observation_override_reason": reason, "planned_at": formatPlanTime(now),
		"workflow_run_id": runID, "workflow_run_attempt": attempt, "repository": repository,
	}, nil
}

func channelComponents(raw document, channel string) document {
	return objectField(objectField(raw, "channels"), channel)
}

func checkStableReleases(repo planningRepo, version string, hours int, now time.Time, reason string) error {
	releases, err := repo.Releases()
	if err != nil {
		return err
	}
	var last time.Time
	for _, raw := range releases {
		var item struct {
			Draft, Prerelease bool
			Tag               string `json:"tag_name"`
			Published         string `json:"published_at"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return err
		}
		if item.Draft || item.Prerelease || !tag.MatchString(item.Tag) || channelFor(item.Tag) != "stable" {
			continue
		}
		if !newer(version, item.Tag) {
			return errors.New("Only versions newer than existing stable versions can be published")
		}
		published, err := time.Parse(time.RFC3339Nano, item.Published)
		if err != nil {
			return errors.New("Invalid publication or verification timestamp")
		}
		if published.After(last) {
			last = published
		}
	}
	if !last.IsZero() {
		return requireAge(last.Format(time.RFC3339Nano), hours, now, reason)
	}
	return nil
}

func formatPlanTime(now time.Time) string {
	now = now.UTC().Truncate(time.Microsecond)
	if now.Nanosecond() == 0 {
		return now.Format("2006-01-02T15:04:05-07:00")
	}
	return now.Format("2006-01-02T15:04:05.000000-07:00")
}
