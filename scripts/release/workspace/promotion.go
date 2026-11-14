package main

import (
	"errors"
	"fmt"

	"github.com/NARUBROWN/spine/scripts/release/internal/sourceproof"
)

func promotion(repo releaseRepository, repository, source, upstream, branch, upstreamBranch, version string) (document, error) {
	var commit struct{ Parents []struct{ SHA string } }
	if err := fetch(repo, "git/commits/"+source, &commit); err != nil {
		return nil, err
	}
	if err := fetch(repo, "git/commits/"+upstream, nil); err != nil {
		return nil, err
	}
	if len(commit.Parents) != 2 {
		return nil, errors.New("Channel promotion requires a merge commit")
	}
	head := commit.Parents[1].SHA
	if head == "" {
		return nil, errors.New("Channel promotion requires a valid merge parent")
	}
	if head != upstream {
		var comparison struct{ Status string }
		if err := fetch(repo, "compare/"+upstream+"..."+head, &comparison); err != nil {
			return nil, err
		}
		if comparison.Status != "ahead" && comparison.Status != "identical" {
			return nil, errors.New("The promotion PR must descend from the verified candidate")
		}
	}
	sourceHash, err := sourceproof.Fingerprint(repo, source)
	if err != nil {
		return nil, err
	}
	upstreamHash, err := sourceproof.Fingerprint(repo, upstream)
	if err != nil {
		return nil, err
	}
	if sourceHash != upstreamHash {
		return nil, errors.New("Promotion includes code changes beyond channel dependencies. Publish a new Preview.")
	}
	var pulls []struct {
		MergedAt string `json:"merged_at"`
		MergeSHA string `json:"merge_commit_sha"`
		Number   int64
		Base     struct{ Ref string }
		Head     struct {
			Ref  string
			Repo struct {
				FullName string `json:"full_name"`
			}
		}
	}
	if err := fetch(repo, "commits/"+source+"/pulls?per_page=100", &pulls); err != nil {
		return nil, err
	}
	channel := branch
	if branch == "main" {
		channel = "stable"
	}
	candidate := "promote/" + channel + "/" + version
	for _, pr := range pulls {
		if pr.MergedAt != "" && pr.MergeSHA == source && pr.Base.Ref == branch &&
			(pr.Head.Ref == upstreamBranch || pr.Head.Ref == candidate) && pr.Head.Repo.FullName == repository {
			return document{"number": pr.Number, "head": upstreamBranch, "base": branch, "merge_sha": source, "head_sha": head, "promotion_branch": pr.Head.Ref}, nil
		}
	}
	return nil, fmt.Errorf("%s → %s requires a merged promotion PR", upstreamBranch, branch)
}
