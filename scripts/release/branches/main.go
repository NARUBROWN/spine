package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/NARUBROWN/spine/scripts/release/internal/approval"
	"github.com/NARUBROWN/spine/scripts/release/internal/releasegithub"
)

type object = map[string]any
type api interface {
	API(string, string, any, any, bool) (bool, error)
}

var branches = []string{"preview", "rc", "main"}

func spec(name, target, ref string, rules []object) object {
	return object{"name": name, "target": target, "enforcement": "active", "bypass_actors": []any{},
		"conditions": object{"ref_name": object{"include": []string{ref}, "exclude": []string{}}}, "rules": rules}
}
func branchSpec(branch string) object {
	methods := []string{"merge"}
	if branch == "preview" {
		methods = append(methods, "squash", "rebase")
	}
	return spec("Spine channel / "+branch, "branch", "refs/heads/"+branch, []object{
		{"type": "deletion"}, {"type": "non_fast_forward"},
		{"type": "pull_request", "parameters": object{
			"allowed_merge_methods": methods, "dismiss_stale_reviews_on_push": true, "require_code_owner_review": false,
			"require_last_push_approval": false, "required_approving_review_count": 0, "required_review_thread_resolution": true}},
		{"type": "required_status_checks", "parameters": object{"strict_required_status_checks_policy": false,
			"required_status_checks": []object{{"context": "Verified Source / " + branch, "integration_id": 15368}, {"context": "Branch Flow / " + branch, "integration_id": 15368}}}},
	})
}
func tagSpec() object {
	return spec("Spine immutable tags", "tag", "refs/tags/v*", []object{{"type": "deletion"}, {"type": "update"}})
}
func verifyRule(actual, expected object) error {
	data, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	var normalized object
	if err := json.Unmarshal(data, &normalized); err != nil {
		return err
	}
	for _, key := range []string{"name", "target", "enforcement", "bypass_actors", "conditions"} {
		if !reflect.DeepEqual(actual[key], normalized[key]) {
			return fmt.Errorf("Branch protection rules do not match: %s / %s", expected["name"], key)
		}
	}
	rules := make(map[string]object)
	actualRules, _ := actual["rules"].([]any)
	for _, value := range actualRules {
		rule, ok := value.(object)
		if !ok {
			return errors.New("잘못된 브랜치 보호 규칙")
		}
		kind, _ := rule["type"].(string)
		rules[kind] = rule
	}
	for _, value := range normalized["rules"].([]any) {
		rule := value.(object)
		kind := rule["type"].(string)
		existing, ok := rules[kind]
		if !ok {
			return fmt.Errorf("Required branch protection rule is missing: %s", kind)
		}
		params, _ := rule["parameters"].(object)
		current, _ := existing["parameters"].(object)
		for key, value := range params {
			if !reflect.DeepEqual(current[key], value) {
				return fmt.Errorf("Branch protection parameters do not match: %s", key)
			}
		}
	}
	return nil
}
func request(g api, method, path string, input, output any) error {
	_, err := g.API(method, path, input, output, false)
	return err
}
func bootstrap(g api, repository string, apply bool) error {
	var ref struct{ Object struct{ SHA string } }
	if err := request(g, "GET", "git/ref/heads/main", nil, &ref); err != nil {
		return err
	}
	for _, branch := range branches[:2] {
		found, err := g.API("GET", "git/ref/heads/"+branch, nil, nil, true)
		if err != nil {
			return err
		}
		if !found {
			fmt.Printf("Initial channel creation plan: %s / %s / %s\n", repository, branch, ref.Object.SHA)
			if apply {
				if err := request(g, "POST", "git/refs", object{"ref": "refs/heads/" + branch, "sha": ref.Object.SHA}, nil); err != nil {
					return err
				}
			}
		}
	}
	fmt.Println("Existing branches will not be moved. Merge the first work PR into preview before applying protection rules.")
	return nil
}

type policy struct {
	Module      string
	Reviewer    string `json:"reviewer_login"`
	Environment string `json:"approval_environment"`
}

type branchPolicy struct {
	Name string
	Type *string
}

func (p branchPolicy) matches(branch string) bool {
	return p.Name == branch && (p.Type == nil || *p.Type == "branch")
}
func branchPolicies(g api, environment string) ([]branchPolicy, error) {
	var result struct {
		Policies []branchPolicy `json:"branch_policies"`
	}
	err := request(g, "GET", "environments/"+environment+"/deployment-branch-policies", nil, &result)
	return result.Policies, err
}
func verifyApproval(g api, p policy) error {
	return approval.Verify(g, p.Environment, p.Reviewer)
}

func configure(g api, apply bool, p policy, lookup func(string) (int64, error), backup func(json.RawMessage) error) error {
	if err := request(g, "GET", "contents/.github/workflows/branch-flow.yml?ref=preview", nil, nil); err != nil {
		return err
	}
	for _, branch := range branches {
		if err := request(g, "GET", "git/ref/heads/"+branch, nil, nil); err != nil {
			return err
		}
	}
	var existing []struct {
		ID   int64
		Name string
	}
	if err := request(g, "GET", "rulesets?per_page=100", nil, &existing); err != nil {
		return err
	}
	ids := make(map[string]int64)
	for _, rule := range existing {
		ids[rule.Name] = rule.ID
	}
	for _, branch := range branches {
		if err := ensureRule(g, ids, branchSpec(branch), apply, "Apply branch protection rules first: "+branch); err != nil {
			return err
		}
		fmt.Println("Channel protection verified:", branch)
	}
	if apply {
		if err := applyEnvironments(g, p, lookup, backup); err != nil {
			return err
		}
	}
	if err := ensureRule(g, ids, tagSpec(), apply, "A rule blocking version tag updates and deletion is required"); err != nil {
		return err
	}
	if err := verifyApproval(g, p); err != nil {
		return err
	}
	var repository struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := request(g, "GET", "", nil, &repository); err != nil {
		return err
	}
	if repository.DefaultBranch != "preview" {
		return errors.New("The repository default branch must be preview")
	}
	found, err := g.API("GET", "branches/main/protection", nil, nil, true)
	if err != nil {
		return err
	}
	if found {
		return errors.New("Legacy main protection remains and may block promotion merges")
	}
	fmt.Println("Default branch and preview → rc → main protection verified")
	return nil
}

func applyEnvironments(g api, p policy, lookup func(string) (int64, error), backup func(json.RawMessage) error) error {
	var legacy json.RawMessage
	found, err := g.API("GET", "branches/main/protection", nil, &legacy, true)
	if err != nil {
		return err
	}
	if found {
		if err := backup(legacy); err != nil {
			return err
		}
		if err := request(g, "DELETE", "branches/main/protection", nil, nil); err != nil {
			return err
		}
	}
	if err := request(g, "PATCH", "", object{"default_branch": "preview"}, nil); err != nil {
		return err
	}
	deployment := object{"protected_branches": false, "custom_branch_policies": true}
	for _, branch := range branches[:2] {
		name := "release-" + branch
		if err := request(g, "PUT", "environments/"+name, object{"can_admins_bypass": false, "deployment_branch_policy": deployment}, nil); err != nil {
			return err
		}
		rules, err := branchPolicies(g, name)
		if err != nil {
			return err
		}
		matched := false
		for _, rule := range rules {
			if rule.matches(branch) {
				matched = true
				break
			}
		}
		if !matched {
			if err := createBranchPolicy(g, name, branch); err != nil {
				return err
			}
		}
	}
	found, err = g.API("GET", "environments/release-stable", nil, nil, true)
	if err != nil || found {
		return err
	}
	id, err := lookup(p.Reviewer)
	if err != nil {
		return err
	}
	payload := object{"can_admins_bypass": false, "prevent_self_review": false, "reviewers": []object{{"type": "User", "id": id}}, "deployment_branch_policy": deployment}
	if err := request(g, "PUT", "environments/release-stable", payload, nil); err != nil {
		return err
	}
	return createBranchPolicy(g, "release-stable", "main")
}

func createBranchPolicy(g api, environment, branch string) error {
	return request(g, "POST", "environments/"+environment+"/deployment-branch-policies", object{"name": branch, "type": "branch"}, nil)
}

func ensureRule(g api, ids map[string]int64, expected object, apply bool, missing string) error {
	id := ids[expected["name"].(string)]
	if apply {
		if id == 0 {
			var created struct{ ID int64 }
			if err := request(g, "POST", "rulesets", expected, &created); err != nil {
				return err
			}
			id = created.ID
		} else if err := request(g, "PUT", fmt.Sprintf("rulesets/%d", id), expected, nil); err != nil {
			return err
		}
	}
	if id == 0 {
		return errors.New(missing)
	}
	var actual object
	if err := request(g, "GET", fmt.Sprintf("rulesets/%d", id), nil, &actual); err != nil {
		return err
	}
	return verifyRule(actual, expected)
}

func backupLegacy(data json.RawMessage) error {
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	formatted, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	raw, err := exec.Command("git", "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return err
	}
	folder := filepath.Join(strings.TrimSpace(string(raw)), "branch-flow-setup")
	if err := os.MkdirAll(folder, 0755); err != nil {
		return err
	}
	path := filepath.Join(folder, "legacy-main-protection.json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = file.Write(append(formatted, '\n'))
	return errors.Join(err, file.Close())
}

func lookup(login string) (int64, error) {
	data, err := exec.Command("gh", "api", "users/"+login).Output()
	if err != nil {
		return 0, err
	}
	var user struct{ ID int64 }
	if err := json.Unmarshal(data, &user); err != nil {
		return 0, err
	}
	if user.ID == 0 {
		return 0, errors.New("지정 리뷰어 ID를 찾을 수 없습니다")
	}
	return user.ID, nil
}
func run() error {
	apply := flag.Bool("apply", false, "apply branch and tag protection")
	initial := flag.Bool("bootstrap", false, "plan or create missing channel branches")
	approval := flag.Bool("check-environment", false, "verify the stable approval environment")
	flag.Parse()
	if flag.NArg() != 0 || (*approval && (*initial || *apply)) {
		return errors.New("unexpected arguments")
	}
	data, err := os.ReadFile(".github/release-policy.json")
	if err != nil {
		return err
	}
	var p policy
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	repository := strings.TrimPrefix(p.Module, "github.com/")
	if current, ok := os.LookupEnv("GITHUB_REPOSITORY"); ok && current != repository {
		return errors.New("The policy repository does not match the current repository")
	}
	github, err := releasegithub.New(repository, "")
	if err != nil {
		return err
	}
	if *approval {
		if err := verifyApproval(github, p); err != nil {
			return err
		}
		fmt.Println("Stable release approval environment verified")
		return nil
	}
	if *initial {
		return bootstrap(github, repository, *apply)
	}
	return configure(github, *apply, p, lookup, backupLegacy)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
