package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/scripts/release/internal/approval"
	"github.com/NARUBROWN/spine/scripts/release/internal/releasegithub"
)

type promotionPlanRepo struct {
	promotionRepo
	prior, source string
	proof         releasegithub.Evidence
	releases      []json.RawMessage
}

type promotionPublicationRepo struct {
	promotionPlanRepo
	writes int
}

func (r *promotionPublicationRepo) API(method, path string, input, output any, optional bool) (bool, error) {
	if method == "POST" && path == "git/refs" {
		r.writes++
		return true, nil
	}
	return r.promotionRepo.API(method, path, input, output, optional)
}

func (r promotionPlanRepo) TagSHA(version string) (string, error) {
	if version == r.prior {
		return r.source, nil
	}
	return "", nil
}

func (r promotionPlanRepo) CandidateEvidence(version, prefix string, optional bool) (*releasegithub.Evidence, error) {
	if version != r.prior || prefix != "release-manifest-" || optional {
		return nil, fmt.Errorf("unexpected evidence lookup: %s%s", prefix, version)
	}
	return &r.proof, nil
}

func (r promotionPlanRepo) Releases() ([]json.RawMessage, error) { return r.releases, nil }

func TestCorePromotionObservationAndSource(t *testing.T) {
	const repository = "NARUBROWN/spine"
	const exception = "서비스 장애 대응을 위한 긴급 수정으로 관찰 기간을 단축합니다"
	now := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	data, err := os.ReadFile("../../../.github/release-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := decodeDocument(data)
	if err != nil {
		t.Fatal(err)
	}
	p, err := decodePolicy(raw)
	if err != nil {
		t.Fatal(err)
	}
	source, target := strings.Repeat("a", 40), strings.Repeat("b", 40)
	for _, tc := range []struct {
		name, channel, priorChannel, reason, failure string
		age                                          int
	}{
		{"early RC", "rc", "preview", "", "Insufficient observation period", 71},
		{"short exception", "rc", "preview", "short", "at least 20 characters", 71},
		{"RC override", "rc", "preview", exception, "", 71},
		{"future evidence", "rc", "preview", exception, "in the future", -1},
		{"normal RC", "rc", "preview", "", "", 72},
		{"early stable", "stable", "rc", "", "Insufficient observation period", 23},
		{"normal stable", "stable", "rc", "", "", 25},
		{"weekly interval", "stable", "rc", "", "Insufficient observation period", 25},
		{"downgrade", "stable", "rc", "", "newer than existing", 25},
		{"null branch type", "stable", "rc", "", "must allow only the main branch", 25},
		{"missing branch type", "stable", "rc", "", "", 25},
		{"missing reviewer", "stable", "rc", "", "requires designated reviewers", 25},
		{"admin bypass", "stable", "rc", "", "requires designated reviewers", 25},
		{"wrong tag SHA", "rc", "preview", "", "does not match the verification manifest", 72},
		{"missing validation", "rc", "preview", "", "does not match the verification manifest", 72},
		{"wrong repository", "rc", "preview", "", "does not match the verification manifest", 72},
		{"wrong tree", "rc", "preview", "", "does not match the verification manifest", 72},
		{"core-only scope", "rc", "preview", "", "does not match the verification manifest", 72},
		{"missing selected module", "rc", "preview", "", "does not match the verification manifest", 72},
		{"stable selected module", "rc", "preview", "", "does not match the verification manifest", 72},
		{"RC selected module", "rc", "preview", "", "does not match the verification manifest", 72},
		{"failed workflow", "rc", "preview", "", "workflow and artifact do not match", 72},
		{"wrong workflow", "rc", "preview", "", "workflow and artifact do not match", 72},
		{"expired artifact", "rc", "preview", "", "workflow and artifact do not match", 72},
		{"wrong artifact run", "rc", "preview", "", "workflow and artifact do not match", 72},
		{"wrong artifact SHA", "rc", "preview", "", "workflow and artifact do not match", 72},
		{"wrong run attempt", "rc", "preview", "", "workflow and artifact do not match", 72},
		{"same base different platform tag", "rc", "preview", "", "Platform tags cannot change", 72},
		{"wrong components", "rc", "preview", "", "does not match the verification manifest", 72},
		{"wrong workflow branch", "rc", "preview", "", "workflow and artifact do not match", 72},
		{"wrong PR direction", "rc", "preview", "", "requires a merged promotion PR", 72},
		{"cannot skip RC", "stable", "preview", "", "requires a rc source tag", 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prior, version := p.Channels[tc.priorChannel].Core, p.Channels[tc.channel].Core
			priorBranch, branch := p.Branches[tc.priorChannel], p.Branches[tc.channel]
			component := channelComponents(raw, tc.priorChannel)
			selected := document{core: prior}
			for module, version := range objectField(component, "modules") {
				selected[module] = version
			}
			manifest := document{
				"schema_version": "spine.release-manifest/v1", "version": prior, "channel": tc.priorChannel,
				"source_sha": source, "source_tree": "tree", "source_fingerprint": fmt.Sprintf("%x", sha256.Sum256([]byte("[]"))),
				"repository": repository, "kind": "core", "module": core, "branch": priorBranch, "components": component,
				"workflow_run_id": "10", "workflow_run_attempt": "2",
				"validation": document{"scope": "cohort", "result": "passed", "source_sha": source, "version": prior, "components": component, "selected_modules": selected},
			}
			proof := releasegithub.Evidence{}
			expired := false
			proof.Artifact.ID, proof.Artifact.Name, proof.Artifact.Expired, proof.Artifact.Digest = 5, "release-manifest-"+prior, &expired, "sha256:fixture"
			proof.Artifact.CreatedAt = now.Add(-time.Duration(tc.age) * time.Hour).Format(time.RFC3339)
			proof.Artifact.WorkflowRun.ID, proof.Artifact.WorkflowRun.HeadSHA, proof.Artifact.WorkflowRun.HeadBranch = 10, source, priorBranch
			switch tc.name {
			case "wrong components":
				manifest["components"] = document{"wrong": "versions"}
			case "missing validation":
				delete(manifest, "validation")
			case "wrong repository":
				manifest["repository"] = "elsewhere/spine"
			case "wrong tree":
				manifest["source_tree"] = strings.Repeat("f", 40)
			case "core-only scope":
				objectField(manifest, "validation")["scope"] = "core"
			case "missing selected module":
				delete(selected, core)
			case "stable selected module":
				selected[core] = "v0.6.5"
			case "RC selected module":
				selected[core] = "v0.7.0-rc.1"
			}
			manifestJSON, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			proof.Manifest = manifestJSON
			run := document{
				"id": 10, "run_attempt": 2, "status": "completed", "conclusion": "success",
				"event": "workflow_dispatch", "head_branch": priorBranch, "head_sha": source,
				"path": ".github/workflows/complete.yml",
			}
			switch tc.name {
			case "wrong workflow branch":
				run["head_branch"] = "main"
			case "failed workflow":
				run["conclusion"] = "failure"
			case "wrong workflow":
				run["path"] = ".github/workflows/ci.yml"
			case "wrong run attempt":
				run["run_attempt"] = 3
			case "expired artifact":
				expired = true
			case "wrong artifact run":
				proof.Artifact.WorkflowRun.ID = 11
			case "wrong artifact SHA":
				proof.Artifact.WorkflowRun.HeadSHA = strings.Repeat("f", 40)
			}
			runJSON, err := json.Marshal(run)
			if err != nil {
				t.Fatal(err)
			}
			proof.WorkflowRun = runJSON
			blob := fmt.Sprintf(`{"content":%q}`, base64.StdEncoding.EncodeToString(data))
			repo := promotionPlanRepo{prior: prior, source: source, proof: proof, promotionRepo: promotionRepo{
				"git/ref/heads/" + branch:                                   fmt.Sprintf(`{"object":{"sha":%q}}`, target),
				"git/matching-refs/tags/" + p.Next + "-" + tc.channel + ".": "[]",
				"git/commits/" + source:                                     `{"tree":{"sha":"tree"}}`,
				"git/commits/" + target:                                     fmt.Sprintf(`{"parents":[{"sha":"previous"},{"sha":%q}],"tree":{"sha":"tree"}}`, source),
				"git/trees/tree?recursive=1":                                `{"tree":[]}`,
				"contents/.github/release-policy.json?ref=" + source:        blob,
				"contents/.github/release-policy.json?ref=" + target:        blob,
				"commits/" + target + "/pulls?per_page=100":                 fmt.Sprintf(`[{"number":1,"merged_at":"2026-11-14","merge_commit_sha":%q,"base":{"ref":%q},"head":{"ref":%q,"repo":{"full_name":%q}}}]`, target, branch, priorBranch, repository),
			}}
			if tc.name == "wrong tag SHA" {
				repo.source = target
			}
			if tc.channel == "rc" {
				repo.releases = []json.RawMessage{json.RawMessage(fmt.Sprintf(`{"tag_name":%q,"published_at":"2026-11-04T00:00:00Z"}`, prior))}
			}

			if tc.channel == "stable" {
				lastTag, days := "v0.6.5", 8
				switch tc.name {
				case "weekly interval":
					days = 2
				case "downgrade":
					lastTag = "v9.0.0"
				}
				repo.releases = []json.RawMessage{json.RawMessage(fmt.Sprintf(`{"tag_name":%q,"published_at":%q}`, lastTag, now.AddDate(0, 0, -days).Format(time.RFC3339)))}
				reviewers := fmt.Sprintf(`[{"type":"User","reviewer":{"login":%q}}]`, p.Reviewer)
				if tc.name == "missing reviewer" {
					reviewers = "[]"
				}
				envPath := "environments/" + p.Environment
				repo.promotionRepo[envPath] = fmt.Sprintf(`{"can_admins_bypass":%t,"deployment_branch_policy":{"custom_branch_policies":true},"protection_rules":[{"type":"required_reviewers","reviewers":%s}]}`, tc.name == "admin bypass", reviewers)
				repo.promotionRepo[envPath+"/deployment-branch-policies"] = `{"branch_policies":[{"name":"main","type":"branch"}]}`
				switch tc.name {
				case "null branch type":
					repo.promotionRepo[envPath+"/deployment-branch-policies"] = `{"branch_policies":[{"name":"main","type":null}]}`
				case "missing branch type":
					repo.promotionRepo[envPath+"/deployment-branch-policies"] = `{"branch_policies":[{"name":"main"}]}`
				}

			}
			if tc.name == "wrong PR direction" {
				path := "commits/" + target + "/pulls?per_page=100"
				repo.promotionRepo[path] = strings.Replace(repo.promotionRepo[path], `"ref":"preview"`, `"ref":"feat/bypass"`, 1)
			}
			planPolicy := raw
			if tc.name == "same base different platform tag" {
				planPolicy, err = decodeDocument(data)
				if err != nil {
					t.Fatal(err)
				}
				pins := objectField(channelComponents(planPolicy, "rc"), "modules")
				for module := range pins {
					pins[module] = strings.Split(textField(pins, module), "-")[0] + "-preview.999"
					break
				}
			}
			if tc.name == "normal RC" || tc.name == "normal stable" {
				report := objectField(manifest, "validation")
				delete(manifest, "validation")
				exported, finishErr := finish(repo, manifest, report, "2", func() (document, error) { return manifest, nil })
				if finishErr != nil {
					t.Fatal(finishErr)
				}
				repo.proof.Manifest, err = json.Marshal(exported)
				if err != nil {
					t.Fatal(err)
				}
			}

			plan, err := makePlan(repo, repository, p, planPolicy, tc.channel, version, prior, tc.reason, "42", "1", now, func() error { return approval.Verify(repo, p.Environment, p.Reviewer) })
			if tc.failure != "" {
				if err == nil || !strings.Contains(err.Error(), tc.failure) {
					t.Fatalf("wanted %q, got %v", tc.failure, err)
				}
				return
			}
			if err != nil || plan["source_sha"] != target || plan["upstream_source_sha"] != source ||
				plan["observation_override_reason"] != tc.reason || objectField(plan, "source_evidence")["created_at"] != proof.Artifact.CreatedAt {
				t.Fatalf("plan %v, error %v", plan, err)
			}
			if tc.name == "normal RC" {
				t.Run("artifact replaced after approval", func(t *testing.T) {
					data, err := json.Marshal(plan)
					if err != nil {
						t.Fatal(err)
					}
					approved, err := decodeDocument(data)
					if err != nil {
						t.Fatal(err)
					}
					repo.promotionRepo["contents/.github/release-policy.json?ref="+branch] = blob
					publishing := &promotionPublicationRepo{promotionPlanRepo: repo}
					publishing.proof.Artifact.ID++
					replan := func(channel, version, source, reason string) (document, error) {
						return makePlan(publishing, repository, p, raw, channel, version, source, reason, "42", "1", now, func() error { return nil })
					}
					err = publish(publishing, approved, raw, target, "refs/heads/"+branch, replan)
					if err == nil || !strings.Contains(err.Error(), "approved plan changed") || publishing.writes != 0 {
						t.Fatalf("tag writes %d, error %v", publishing.writes, err)
					}
				})
			}

		})
	}
}
