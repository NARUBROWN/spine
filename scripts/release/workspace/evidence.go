package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/NARUBROWN/spine/scripts/release/internal/releasegithub"
	"github.com/NARUBROWN/spine/scripts/release/internal/sourceproof"
)

type releaseRepository interface {
	sourceproof.API
	TagSHA(string) (string, error)
	CandidateEvidence(string, string, bool) (*releasegithub.Evidence, error)
}

type document = map[string]any

func decodeDocument(raw []byte) (document, error) {
	var value document
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if !json.Valid(raw) || value == nil {
		return nil, errors.New("expected a JSON object")
	}
	return value, nil
}

func textField(doc document, key string) string {
	value, _ := doc[key].(string)
	return value
}
func objectField(doc document, key string) document {
	value, _ := doc[key].(map[string]any)
	return value
}
func fetch(repo releaseRepository, path string, out any) error {
	_, err := repo.API("GET", path, nil, out, false)
	return err
}
func policyAt(repo releaseRepository, ref string) (document, error) {
	var blob struct{ Content string }
	if err := fetch(repo, "contents/.github/release-policy.json?ref="+ref, &blob); err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(blob.Content)
	if err != nil {
		return nil, err
	}
	return decodeDocument(raw)
}

func decodePolicy(doc document) (policy, error) {
	var result policy
	raw, err := json.Marshal(doc)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	return result, result.validate()
}

func selectedModulesMatch(candidate, report document, scope string) bool {
	component := objectField(candidate, "components")
	selected := objectField(report, "selected_modules")
	expected := map[string]any{core: component["core_version"]}
	if scope == "cohort" {
		for module, version := range objectField(component, "modules") {
			expected[module] = version
		}
	} else if scope == "platform" {
		expected[textField(candidate, "module")] = candidate["version"]
	}
	for module, version := range expected {
		if version == nil || !reflect.DeepEqual(selected[module], version) {
			return false
		}
	}
	return true
}

func verifyCandidate(repo releaseRepository, repository, version, channel string, p policy, bootstrap bool) (releasegithub.Artifact, document, error) {
	var empty releasegithub.Artifact
	if !tag.MatchString(version) || channelFor(version) != channel {
		return empty, nil, fmt.Errorf("This stage requires a %s source tag", channel)
	}
	prefix := "release-manifest-"
	if bootstrap {
		prefix = "core-candidate-"
	}
	evidence, err := repo.CandidateEvidence(version, prefix, false)
	if err != nil {
		return empty, nil, err
	}
	if evidence == nil {
		return empty, nil, errors.New("candidate verification evidence missing")
	}
	source, err := repo.TagSHA(version)
	if err != nil {
		return empty, nil, err
	}
	if !sha.MatchString(source) {
		return empty, nil, errors.New("The candidate tag has no valid source SHA")
	}
	tagged, err := policyAt(repo, source)
	if err != nil {
		return empty, nil, err
	}
	if _, err := decodePolicy(tagged); err != nil {
		return empty, nil, err
	}
	manifest, err := decodeDocument(evidence.Manifest)
	if err != nil {
		return empty, nil, err
	}
	validation := objectField(manifest, "validation")
	scope := "platform"
	if bootstrap {
		scope = "core"
	} else if p.Kind == "core" {
		scope = "cohort"
	}
	branch := p.Branches[channel]
	taggedChannel := objectField(objectField(tagged, "channels"), channel)
	mismatch := errors.New("Source tag does not match the verification manifest")
	if textField(manifest, "schema_version") != "spine.release-manifest/v1" || textField(manifest, "version") != version ||
		textField(manifest, "channel") != channel || textField(manifest, "source_sha") != source ||
		textField(manifest, "repository") != repository || textField(manifest, "kind") != p.Kind ||
		textField(manifest, "module") != p.Module || textField(manifest, "branch") != branch {
		return empty, nil, mismatch
	}
	tree, err := sourceproof.Tree(repo, source)
	if err != nil {
		return empty, nil, err
	}
	if textField(manifest, "source_tree") != tree {
		return empty, nil, mismatch
	}
	fingerprint, err := sourceproof.Fingerprint(repo, source)
	if err != nil {
		return empty, nil, err
	}
	if textField(manifest, "source_fingerprint") != fingerprint || textField(validation, "scope") != scope ||
		textField(validation, "result") != "passed" || !reflect.DeepEqual(manifest["components"], taggedChannel) ||
		!selectedModulesMatch(manifest, validation, scope) || textField(validation, "source_sha") != source ||
		textField(validation, "version") != version || !reflect.DeepEqual(validation["components"], manifest["components"]) {
		return empty, nil, mismatch
	}
	var run struct {
		ID                              int64
		Attempt                         int64 `json:"run_attempt"`
		Status, Conclusion, Event, Path string
		HeadSHA                         string `json:"head_sha"`
		HeadBranch                      string `json:"head_branch"`
	}
	if err := json.Unmarshal(evidence.WorkflowRun, &run); err != nil {
		return empty, nil, err
	}
	artifact := evidence.Artifact
	workflow := map[string]string{"preview": "preview.yml", "rc": "rc.yml", "stable": "release.yml"}[channel]
	if p.Kind == "core" && !bootstrap {
		workflow = "complete.yml"
	}
	path, _, _ := strings.Cut(run.Path, "@")
	if run.Conclusion != "success" || run.Status != "completed" || run.Event != "workflow_dispatch" ||
		run.HeadBranch != branch || run.HeadSHA != source || strconv.FormatInt(run.ID, 10) != textField(manifest, "workflow_run_id") ||
		strconv.FormatInt(run.Attempt, 10) != textField(manifest, "workflow_run_attempt") || artifact.Name != prefix+version ||
		artifact.Expired == nil || *artifact.Expired || artifact.WorkflowRun.ID != run.ID ||
		artifact.WorkflowRun.HeadSHA != source || artifact.WorkflowRun.HeadBranch != branch || path != ".github/workflows/"+workflow {
		return empty, nil, errors.New("Source tag verification workflow and artifact do not match or verification did not succeed")
	}
	return artifact, manifest, nil
}

func completePlan(repo releaseRepository, repository string, p policy, raw document, version, ref, runID, attempt string, peerTag func(string, string) (string, error)) (document, error) {
	match := tag.FindStringSubmatch(version)
	if match == nil || p.Kind != "core" || match[1] != p.Next {
		return nil, errors.New("Component verification completion must run from the core repository for that version")
	}
	channel := channelFor(version)
	artifact, candidate, err := verifyCandidate(repo, repository, version, channel, p, true)
	if err != nil {
		return nil, err
	}
	branch := p.Branches[channel]
	var head struct{ Object struct{ SHA string } }
	if err := fetch(repo, "git/ref/heads/"+branch, &head); err != nil {
		return nil, err
	}
	current, err := policyAt(repo, head.Object.SHA)
	if err != nil {
		return nil, err
	}
	if ref != "refs/heads/"+branch || head.Object.SHA != textField(candidate, "source_sha") ||
		!reflect.DeepEqual(candidate["components"], objectField(objectField(raw, "channels"), channel)) || !reflect.DeepEqual(current, raw) {
		return nil, errors.New("Channel HEAD or component versions changed after the core candidate was published")
	}
	peers := make(map[string]map[string]string)
	for _, suffix := range platforms {
		module := core + "-" + suffix
		version := p.Channels[channel].Modules[module]
		source, err := peerTag(module, version)
		if err != nil {
			return nil, err
		}
		if !sha.MatchString(source) {
			return nil, fmt.Errorf("Platform candidate tag is not available yet: %s", module)
		}
		peers[module] = map[string]string{"version": version, "source_sha": source}
	}
	candidate["bootstrap_evidence"] = map[string]any{"artifact_id": artifact.ID, "digest": artifact.Digest}
	candidate["platform_evidence"] = peers
	candidate["workflow_run_id"] = runID
	candidate["workflow_run_attempt"] = attempt
	return candidate, nil
}
