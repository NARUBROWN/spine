package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type publicationFixture struct {
	previewPlanRepo
	branchPolicy string
	writes       []document
}

func (r *publicationFixture) API(method, path string, input, output any, optional bool) (bool, error) {
	if method == "GET" && path == "contents/.github/release-policy.json?ref=preview" {
		payload, err := json.Marshal(document{"content": base64.StdEncoding.EncodeToString([]byte(r.branchPolicy))})
		if err != nil {
			return false, err
		}
		return true, json.Unmarshal(payload, output)
	}
	if method == "POST" && path == "git/refs" && !optional {
		r.writes = append(r.writes, input.(document))
		return true, nil
	}
	return false, errors.New("unexpected request: " + method + " " + path)
}

func TestPublishChecksApprovedTargetBeforeWriting(t *testing.T) {
	const version = "v0.7.5-preview.1"
	source := strings.Repeat("a", 40)
	const rawPolicy = `{"preview_hours":72}`
	raw, err := decodeDocument([]byte(rawPolicy))
	if err != nil {
		t.Fatal(err)
	}
	approved := document{"source_sha": source, "branch": "preview", "channel": "preview", "version": version, "source_evidence": document{"artifact_id": json.Number("10")}}
	for _, tc := range []struct{ name, want string }{
		{"approved", ""},
		{"wrong SHA", "verified commit"},
		{"wrong branch", "workflow branch"},
		{"changed policy", "branch release policy changed"},
		{"head moved", "preview branch HEAD"},
		{"changed artifact", "approved plan changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &publicationFixture{branchPolicy: rawPolicy}
			verified, ref := source, "refs/heads/preview"
			switch tc.name {
			case "wrong SHA":
				verified = strings.Repeat("b", 40)
			case "wrong branch":
				ref = "refs/heads/main"
			case "changed policy":
				repo.branchPolicy = `{"preview_hours":73}`
			}
			called := false
			replan := func(channel, tag, target, reason string) (document, error) {
				called = true
				if channel != "preview" || tag != version || target != source || reason != "" {
					t.Fatalf("unexpected replan: %s %s %s %s", channel, tag, target, reason)
				}
				fresh := maps.Clone(approved)
				switch tc.name {
				case "head moved":
					return nil, errors.New("Preview source requires the full commit SHA of the preview branch HEAD")
				case "changed artifact":
					fresh["source_evidence"] = document{"artifact_id": 11}
				}
				return fresh, nil
			}
			err := publish(repo, approved, raw, verified, ref, replan)
			if tc.want == "" {
				want := []document{{"ref": "refs/tags/" + version, "sha": source}}
				if err != nil || !called || !reflect.DeepEqual(repo.writes, want) {
					t.Fatalf("writes=%v replan=%t err=%v", repo.writes, called, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) || len(repo.writes) != 0 {
				t.Fatalf("writes=%v err=%v want %s", repo.writes, err, tc.want)
			}
		})
	}
}

func TestPublicationOutputsMatchLegacyWorkflow(t *testing.T) {
	for _, version := range []string{"v0.7.5-preview.1", "v0.7.5-rc.1", "v0.7.5"} {
		t.Run(version, func(t *testing.T) {
			root := t.TempDir()
			output, summary := filepath.Join(root, "output"), filepath.Join(root, "summary")
			source := strings.Repeat("a", 40)
			if err := os.WriteFile(output, []byte("previous=value\n"), 0600); err != nil {
				t.Fatal(err)
			}
			plan := document{"source_sha": source, "version": version, "kind": "core"}
			if err := writePublicationOutputs(plan, output, summary); err != nil {
				t.Fatal(err)
			}
			for path, want := range map[string]string{
				output:  "previous=value\nsha=" + source + "\nversion=" + version + "\nkind=core\n",
				summary: "Release plan: `" + version + "` / `" + source + "`\n\nObservation period override reason: none\n",
			} {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != want {
					t.Fatalf("%s: %q, want %q, err %v", path, got, want, err)
				}
			}
		})
	}
}
