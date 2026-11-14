package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/scripts/release/internal/releasegithub"
)

type completionRepo struct {
	source string
	policy []byte
	proof  releasegithub.Evidence
	head   string
}

func (r completionRepo) TagSHA(string) (string, error) { return r.source, nil }
func (r completionRepo) CandidateEvidence(version, prefix string, optional bool) (*releasegithub.Evidence, error) {
	if prefix != "core-candidate-" || optional || r.proof.Artifact.Name != prefix+version {
		return nil, fmt.Errorf("unexpected evidence request")
	}
	return &r.proof, nil
}
func (r completionRepo) API(method, path string, input, output any, optional bool) (bool, error) {
	if method != "GET" || optional {
		return false, fmt.Errorf("unexpected request %s %s", method, path)
	}
	var response string
	switch path {
	case "git/ref/heads/preview":
		head := r.head
		if head == "" {
			head = r.source
		}
		response = fmt.Sprintf(`{"object":{"sha":%q}}`, head)
	case "git/commits/" + r.source:
		response = `{"tree":{"sha":"tree"}}`
	case "git/trees/tree?recursive=1":
		response = `{"tree":[]}`
	case "contents/.github/release-policy.json?ref=" + r.source, "contents/.github/release-policy.json?ref=" + r.head:
		response = fmt.Sprintf(`{"content":%q}`, base64.StdEncoding.EncodeToString(r.policy))
	default:
		return false, fmt.Errorf("unexpected request %s", path)
	}
	return true, json.Unmarshal([]byte(response), output)
}
func TestCompletePlanConfirmsCandidateAndAllPlatformTags(t *testing.T) {
	raw, err := os.ReadFile("../../../.github/release-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := decodeDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	p, err := decodePolicy(doc)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Repeat("a", 40)
	version := p.Channels["preview"].Core
	components := objectField(objectField(doc, "channels"), "preview")
	manifest := document{
		"schema_version": "spine.release-manifest/v1", "version": version, "channel": "preview",
		"source_sha": source, "repository": "NARUBROWN/spine", "kind": "core", "module": core, "branch": "preview",
		"source_tree": "tree", "source_fingerprint": fmt.Sprintf("%x", sha256.Sum256([]byte("[]"))),
		"workflow_run_id": "10", "workflow_run_attempt": "2", "components": components,
		"validation": document{"scope": "core", "result": "passed", "source_sha": source, "version": version, "components": components, "selected_modules": map[string]string{core: version}},
	}
	proof := releasegithub.Evidence{}
	expired := false
	proof.Artifact.ID, proof.Artifact.Name, proof.Artifact.Expired, proof.Artifact.Digest = 5, "core-candidate-"+version, &expired, "sha256:fixture"
	proof.Artifact.WorkflowRun.ID, proof.Artifact.WorkflowRun.HeadSHA, proof.Artifact.WorkflowRun.HeadBranch = 10, source, "preview"
	proof.Manifest, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	proof.WorkflowRun = []byte(fmt.Sprintf(`{"id":10,"run_attempt":2,"status":"completed","conclusion":"success","event":"workflow_dispatch","head_branch":"preview","head_sha":%q,"path":".github/workflows/preview.yml@refs/heads/preview"}`, source))
	repo := completionRepo{source: source, policy: raw, proof: proof}
	for _, scenario := range []string{
		"valid", "moved head", "wrong ref", "missing peer", "wrong attempt", "failed run", "wrong workflow",
		"wrong tag SHA", "missing validation", "expired artifact", "foreign manifest", "wrong artifact run",
		"wrong artifact SHA", "wrong artifact branch", "wrong tree", "wrong fingerprint", "wrong scope",
		"missing selected core", "wrong selected core", "source query report",
	} {
		t.Run(scenario, func(t *testing.T) {
			repo := repo
			ref := "refs/heads/preview"
			manifest, err := decodeDocument(repo.proof.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			run, err := decodeDocument(repo.proof.WorkflowRun)
			if err != nil {
				t.Fatal(err)
			}
			validation := objectField(manifest, "validation")
			switch scenario {
			case "moved head":
				repo.head = strings.Repeat("c", 40)
			case "wrong ref":
				ref = "refs/heads/main"
			case "wrong attempt":
				run["run_attempt"] = 3
			case "failed run":
				run["conclusion"] = "failure"
			case "wrong workflow":
				run["path"] = ".github/workflows/ci.yml"
			case "wrong tag SHA":
				repo.source = strings.Repeat("c", 40)
			case "missing validation":
				delete(manifest, "validation")
			case "expired artifact":
				expired := true
				repo.proof.Artifact.Expired = &expired
			case "foreign manifest":
				manifest["repository"] = "other/spine"
			case "wrong artifact run":
				repo.proof.Artifact.WorkflowRun.ID = 11
			case "wrong artifact SHA":
				repo.proof.Artifact.WorkflowRun.HeadSHA = strings.Repeat("c", 40)
			case "wrong artifact branch":
				repo.proof.Artifact.WorkflowRun.HeadBranch = "rc"
			case "wrong tree":
				manifest["source_tree"] = "wrong"
			case "wrong fingerprint":
				manifest["source_fingerprint"] = "wrong"
			case "wrong scope":
				validation["scope"] = "cohort"
			case "missing selected core":
				delete(validation, "selected_modules")
			case "wrong selected core":
				objectField(validation, "selected_modules")[core] = "v0.6.5"
			case "source query report":
				validation["version"] = source
			}
			repo.proof.Manifest, err = json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			repo.proof.WorkflowRun, err = json.Marshal(run)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			peer := func(module, version string) (string, error) {
				calls++
				if p.Channels["preview"].Modules[module] != version {
					t.Fatal(module, version)
				}
				if scenario == "missing peer" {
					return "", nil
				}
				return strings.Repeat("b", 40), nil
			}
			result, err := completePlan(repo, "NARUBROWN/spine", p, doc, version, ref, "42", "3", peer)
			if scenario != "valid" {
				if err == nil || result != nil || (scenario != "missing peer" && calls != 0) {
					t.Fatalf("%v %v calls=%d", result, err, calls)
				}
				return
			}
			if err != nil || calls != 7 || textField(result, "workflow_run_id") != "42" || textField(result, "workflow_run_attempt") != "3" {
				t.Fatalf("%v %v calls=%d", result, err, calls)
			}
			peers := result["platform_evidence"].(map[string]map[string]string)
			if len(peers) != len(p.Channels["preview"].Modules) {
				t.Fatal(peers)
			}
			for module, version := range p.Channels["preview"].Modules {
				peer := peers[module]
				if len(peer) != 2 || peer["version"] != version || peer["source_sha"] != strings.Repeat("b", 40) {
					t.Fatalf("%s: %v", module, peer)
				}
			}
		})
	}
}
