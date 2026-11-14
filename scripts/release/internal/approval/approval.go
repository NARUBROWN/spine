package approval

import (
	"encoding/json"
	"errors"

	"github.com/NARUBROWN/spine/scripts/release/internal/sourceproof"
)

func Verify(g sourceproof.API, name, reviewerLogin string) error {
	var env struct {
		Bypass       *bool `json:"can_admins_bypass"`
		BranchPolicy struct {
			Custom bool `json:"custom_branch_policies"`
		} `json:"deployment_branch_policy"`
		Rules []struct {
			Type      string
			Reviewers []struct {
				Type     string
				Reviewer struct{ Login string }
			}
		} `json:"protection_rules"`
	}
	if _, err := g.API("GET", "environments/"+name, nil, &env, false); err != nil {
		return err
	}
	reviewer := false
	for _, rule := range env.Rules {
		if rule.Type != "required_reviewers" {
			continue
		}
		for _, user := range rule.Reviewers {
			if user.Type == "User" && user.Reviewer.Login == reviewerLogin {
				reviewer = true
			}
		}
	}
	if !reviewer || env.Bypass == nil || *env.Bypass || !env.BranchPolicy.Custom {
		return errors.New("The stable approval environment requires designated reviewers, no bypass and branch restrictions")
	}
	var result struct {
		Policies []struct {
			Name string
			Type json.RawMessage
		} `json:"branch_policies"`
	}
	if _, err := g.API("GET", "environments/"+name+"/deployment-branch-policies", nil, &result, false); err != nil {
		return err
	}
	for _, rule := range result.Policies {
		kind := "branch"
		if rule.Type != nil {
			kind = ""
			if err := json.Unmarshal(rule.Type, &kind); err != nil {
				continue
			}
		}
		if rule.Name == "main" && kind == "branch" {
			return nil
		}
	}
	return errors.New("The stable approval environment must allow only the main branch")
}
