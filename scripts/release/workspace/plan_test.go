package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type previewPlanRepo struct {
	completionRepo
	existing string
}

func (r previewPlanRepo) TagSHA(version string) (string, error) {
	if version == r.existing {
		return r.source, nil
	}
	return "", nil
}
func (r previewPlanRepo) Releases() ([]json.RawMessage, error) { return nil, nil }
func (r previewPlanRepo) API(method, path string, input, output any, optional bool) (bool, error) {
	if strings.HasPrefix(path, "git/matching-refs/tags/") {
		refs := []document{}
		if r.existing != "" {
			refs = append(refs, document{"ref": "refs/tags/" + r.existing})
		}
		data, err := json.Marshal(refs)
		if err != nil {
			return false, err
		}
		return true, json.Unmarshal(data, output)
	}
	return r.completionRepo.API(method, path, input, output, optional)
}

func TestPreviewPlanRequiresCurrentSource(t *testing.T) {
	source := strings.Repeat("a", 40)
	p := policy{Module: core, Kind: "core", Next: "v0.7.5", Branches: map[string]string{"preview": "preview"}}
	raw := document{"channels": document{"preview": document{"core_version": "v0.7.5-preview.1"}}}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	repo := previewPlanRepo{completionRepo: completionRepo{source: source, policy: data}}
	approved := func() error { t.Fatal("unexpected approval check"); return nil }
	now := time.Date(2026, 11, 14, 0, 0, 0, 123000000, time.UTC)
	output, err := makePlan(repo, "NARUBROWN/spine", p, raw, "preview", "v0.7.5-preview.1", source, "", "42", "1", now, approved)
	if err != nil || output["source_sha"] != source || output["source_tree"] != "tree" || output["planned_at"] != "2026-11-14T00:00:00.123000+00:00" {
		t.Fatalf("%v %v", output, err)
	}
	for _, invalid := range []string{"main", "HEAD", "--help", source[:12], strings.Repeat("b", 40)} {
		if _, err := makePlan(repo, "NARUBROWN/spine", p, raw, "preview", "v0.7.5-preview.1", invalid, "", "42", "1", now, approved); err == nil {
			t.Fatalf("accepted source %q", invalid)
		}
	}

	for _, invalid := range []string{"v0.7.5", "v0.7.5-rc.1", "v9.9.9-preview.1", "v0.7.5-preview.01", "v01.2.3-preview.1"} {
		if _, err := makePlan(repo, "NARUBROWN/spine", p, raw, "preview", invalid, source, "", "42", "1", now, approved); err == nil {
			t.Fatalf("accepted version %q", invalid)
		}
	}

	repo.existing = "v0.7.5-preview.3"
	for _, version := range []string{"v0.7.5-preview.1", "v0.7.5-preview.3", "v0.7.5-preview.4"} {
		_, err := makePlan(repo, "NARUBROWN/spine", p, raw, "preview", version, source, "", "42", "1", now, approved)
		if (err == nil) != (version == "v0.7.5-preview.4") {
			t.Fatalf("version %s with existing %s: %v", version, repo.existing, err)
		}
	}

	repo.existing = ""
	repo.policy = []byte(`{"next_version":"v0.6.5"}`)
	if _, err := makePlan(repo, "NARUBROWN/spine", p, raw, "preview", "v0.7.5-preview.1", source, "", "42", "1", now, approved); err == nil {
		t.Fatal("accepted source with outdated policy")
	}
}

func TestPlanTimeMatchesLegacyISOFormat(t *testing.T) {
	at := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	for offset, want := range map[time.Duration]string{
		0:                "2026-11-14T00:00:00+00:00",
		time.Microsecond: "2026-11-14T00:00:00.000001+00:00",
	} {
		if got := formatPlanTime(at.Add(offset)); got != want {
			t.Fatalf("timestamp %q, want %q", got, want)
		}
	}
}
