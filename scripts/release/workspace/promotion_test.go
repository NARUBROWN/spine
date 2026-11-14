package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/scripts/release/internal/releasegithub"
)

type promotionRepo map[string]string

func (r promotionRepo) API(method, path string, input, output any, optional bool) (bool, error) {
	body, ok := r[path]
	if method != "GET" || !ok || optional {
		return false, fmt.Errorf("unexpected request %s %s", method, path)
	}
	if output == nil {
		return true, nil
	}
	return true, json.Unmarshal([]byte(body), output)
}
func (r promotionRepo) TagSHA(string) (string, error) { return "", fmt.Errorf("unexpected tag lookup") }
func (r promotionRepo) CandidateEvidence(string, string, bool) (*releasegithub.Evidence, error) {
	return nil, fmt.Errorf("unexpected evidence lookup")
}

func TestPromotionProvenance(t *testing.T) {
	const pullPath = "commits/merged/pulls?per_page=100"
	for _, scenario := range []string{"valid", "descendant", "single parent", "diverged", "changed tree", "foreign PR"} {
		t.Run(scenario, func(t *testing.T) {
			parent := "upstream"
			if scenario == "descendant" || scenario == "diverged" {
				parent = "descendant"
			}
			comparison := "ahead"
			if scenario == "diverged" {
				comparison = "diverged"
			}
			pullRepo := "NARUBROWN/spine"
			if scenario == "foreign PR" {
				pullRepo = "other/spine"
			}
			repo := promotionRepo{
				"git/commits/merged":                  fmt.Sprintf(`{"parents":[{"sha":"initial"},{"sha":%q}],"tree":{"sha":"merged-tree"}}`, parent),
				"git/commits/upstream":                `{"tree":{"sha":"upstream-tree"}}`,
				"compare/upstream...descendant":       fmt.Sprintf(`{"status":%q}`, comparison),
				"git/trees/merged-tree?recursive=1":   `{"tree":[{"path":"main.go","mode":"100644","type":"blob","sha":"same"}]}`,
				"git/trees/upstream-tree?recursive=1": `{"tree":[{"path":"main.go","mode":"100644","type":"blob","sha":"same"}]}`,
				pullPath:                              fmt.Sprintf(`[{"number":1,"merged_at":"2026-11-14","merge_commit_sha":"merged","base":{"ref":"main"},"head":{"ref":"promote/stable/v0.7.5","repo":{"full_name":%q}}}]`, pullRepo),
			}
			if scenario == "single parent" {
				repo["git/commits/merged"] = `{"parents":[{"sha":"initial"}]}`
			}
			if scenario == "changed tree" {
				repo["git/trees/merged-tree?recursive=1"] = strings.ReplaceAll(repo["git/trees/merged-tree?recursive=1"], "same", "changed")
			}
			result, err := promotion(repo, "NARUBROWN/spine", "merged", "upstream", "main", "rc", "v0.7.5")
			want := map[string]string{"single parent": "requires a merge commit", "diverged": "must descend", "changed tree": "includes code changes", "foreign PR": "requires a merged promotion PR"}[scenario]
			if want != "" {
				if err == nil || !strings.Contains(err.Error(), want) || result != nil {
					t.Fatalf("%v %v", result, err)
				}
				return
			}
			if err != nil || result["head_sha"] != parent || result["number"] != int64(1) || result["promotion_branch"] != "promote/stable/v0.7.5" {
				t.Fatalf("%v %v", result, err)
			}
		})
	}
}
