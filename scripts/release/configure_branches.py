#!/usr/bin/env python3
"""채널 브랜치의 GitHub 보호 규칙을 확인하거나 명시적으로 적용합니다."""

import argparse
import json
import os
from pathlib import Path

from release import GitHub, ReleaseError, run, approval_environment


BRANCHES = ("preview", "rc", "main")


def bootstrap(github, apply=False):
    source = github.api("git/ref/heads/main")["object"]["sha"]
    for branch in ("preview", "rc"):
        if github.api("git/ref/heads/" + branch, missing=True) is None:
            print(f"Initial channel creation plan: {github.repository} / {branch} / {source}")
            if apply:
                github.api("git/refs", method="POST", data={"ref": "refs/heads/" + branch, "sha": source})
    print("Existing branches will not be moved. Merge the first work PR into preview before applying protection rules.")


def specification(branch):
    return {
        "name": "Spine channel / " + branch,
        "target": "branch",
        "enforcement": "active",
        "bypass_actors": [],
        "conditions": {"ref_name": {"include": ["refs/heads/" + branch], "exclude": []}},
        "rules": [
            {"type": "deletion"},
            {"type": "non_fast_forward"},
            {"type": "pull_request", "parameters": {
                "allowed_merge_methods": ["merge", "squash", "rebase"] if branch == "preview" else ["merge"],
                "dismiss_stale_reviews_on_push": True,
                "require_code_owner_review": False,
                "require_last_push_approval": False,
                "required_approving_review_count": 0,
                "required_review_thread_resolution": True,
            }},
            {"type": "required_status_checks", "parameters": {
                "strict_required_status_checks_policy": False,
                "required_status_checks": [
                    {"context": "Verified Source / " + branch, "integration_id": 15368},
                    {"context": "Branch Flow / " + branch, "integration_id": 15368},
                ],
            }},
        ],
    }


def verify_rule(actual, expected):
    for key in ("name", "target", "enforcement", "bypass_actors", "conditions"):
        if actual.get(key) != expected[key]:
            raise ReleaseError(f"Branch protection rules do not match: {expected['name']} / {key}")
    rules = {rule["type"]: rule for rule in actual["rules"]}
    for rule in expected["rules"]:
        if rule["type"] not in rules:
            raise ReleaseError("Required branch protection rule is missing: " + rule["type"])
        params = rules[rule["type"]].get("parameters", {})
        for key, value in rule.get("parameters", {}).items():
            if params.get(key) != value:
                raise ReleaseError("Branch protection parameters do not match: " + key)


def configure(github, apply=False, policy=None):
    # 기본 브랜치를 바꾸기 전에 신뢰할 PR 방향 검사 코드가 preview에 있어야 합니다.
    github.api("contents/.github/workflows/branch-flow.yml?ref=preview")
    for branch in BRANCHES:
        github.api("git/ref/heads/"+branch)
    listed = {rule["name"]: rule for rule in github.api("rulesets?per_page=100")}
    for branch in BRANCHES:
        expected = specification(branch)
        previous = listed.get(expected["name"])
        if apply:
            if previous:
                github.api(f"rulesets/{previous['id']}", method="PUT", data=expected)
            else:
                previous = github.api("rulesets", method="POST", data=expected)
        if not previous:
            raise ReleaseError("Apply branch protection rules first: " + branch)
        verify_rule(github.api(f"rulesets/{previous['id']}"), expected)
        print("Channel protection verified:", branch)
    if apply:
        # 새 규칙이 활성화된 다음에만 merge commit을 금지하던 이전 main 보호를 제거합니다.
        legacy = github.api("branches/main/protection", missing=True)
        if legacy is not None:
            common = Path(run("git", "rev-parse", "--git-common-dir")).resolve()
            backup = common / "branch-flow-setup"
            backup.mkdir(exist_ok=True)
            path = backup / "legacy-main-protection.json"
            if not path.exists():
                path.write_text(json.dumps(legacy, ensure_ascii=False, indent=2)+"\n")
            github.api("branches/main/protection", method="DELETE")
        github.api("", method="PATCH", data={"default_branch": "preview"})
        for branch in ("preview", "rc"):
            environment = "release-"+branch
            github.api("environments/"+environment, method="PUT", data={
                "can_admins_bypass": False,
                "deployment_branch_policy": {"protected_branches": False, "custom_branch_policies": True},
            })
            policies = github.api(f"environments/{environment}/deployment-branch-policies")["branch_policies"]
            if not any(p["name"] == branch and p.get("type", "branch") == "branch" for p in policies):
                github.api(f"environments/{environment}/deployment-branch-policies", method="POST",
                           data={"name": branch, "type": "branch"})
        # 하위 저장소에도 정식 발행 전 지정 리뷰어 승인이 있어야 합니다.
        if policy is not None and github.api("environments/release-stable", missing=True) is None:
            reviewer = json.loads(run("gh", "api", "users/" + policy["reviewer_login"]))
            github.api("environments/release-stable", method="PUT", data={
                "can_admins_bypass": False, "prevent_self_review": False,
                "reviewers": [{"type": "User", "id": reviewer["id"]}],
                "deployment_branch_policy": {"protected_branches": False, "custom_branch_policies": True}})
            github.api("environments/release-stable/deployment-branch-policies", method="POST",
                       data={"name": "main", "type": "branch"})
    expected_tags = {"name": "Spine immutable tags", "target": "tag", "enforcement": "active", "bypass_actors": [],
                     "conditions": {"ref_name": {"include": ["refs/tags/v*"], "exclude": []}},
                     "rules": [{"type": "deletion"}, {"type": "update"}]}
    tags = listed.get(expected_tags["name"])
    if apply:
        tags = github.api("rulesets" + ("/" + str(tags["id"]) if tags else ""),
                          method="PUT" if tags else "POST", data=expected_tags)
    if not tags:
        raise ReleaseError("A rule blocking version tag updates and deletion is required")
    verify_rule(github.api(f"rulesets/{tags['id']}"), expected_tags)
    if policy is not None:
        approval_environment(github, policy)
    repo = github.api("")
    if repo["default_branch"] != "preview":
        raise ReleaseError("The repository default branch must be preview")
    if github.api("branches/main/protection", missing=True) is not None:
        raise ReleaseError("Legacy main protection remains and may block promotion merges")
    print("Default branch and preview → rc → main protection verified")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description='Configure and verify release channel branch protection.')
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--bootstrap", action="store_true")
    args = parser.parse_args()
    policy = json.loads(Path(".github/release-policy.json").read_text())
    repository = policy["module"].removeprefix("github.com/")
    if os.environ.get("GITHUB_REPOSITORY", repository) != repository:
        raise ReleaseError("The policy repository does not match the current repository")
    if args.bootstrap:
        bootstrap(GitHub(repository), args.apply)
    else:
        configure(GitHub(repository), args.apply, policy)
