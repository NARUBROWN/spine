#!/usr/bin/env python3
"""채널 브랜치의 GitHub 보호 규칙을 확인하거나 명시적으로 적용합니다."""

import argparse
import json
import os
from pathlib import Path

from release import GitHub, ReleaseError, run


BRANCHES = ("preview", "rc", "main")


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
            raise ReleaseError(f"브랜치 보호 규칙 불일치: {expected['name']} / {key}")
    rules = {rule["type"]: rule for rule in actual["rules"]}
    for rule in expected["rules"]:
        if rule["type"] not in rules:
            raise ReleaseError("필수 브랜치 보호 규칙이 없습니다: " + rule["type"])
        params = rules[rule["type"]].get("parameters", {})
        for key, value in rule.get("parameters", {}).items():
            if params.get(key) != value:
                raise ReleaseError("브랜치 보호 매개변수가 다릅니다: " + key)


def configure(github, apply=False):
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
            raise ReleaseError("브랜치 보호 규칙을 먼저 적용해야 합니다: " + branch)
        verify_rule(github.api(f"rulesets/{previous['id']}"), expected)
        print("채널 보호 확인:", branch)
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
    repo = github.api("")
    if repo["default_branch"] != "preview":
        raise ReleaseError("저장소 기본 브랜치는 preview여야 합니다")
    if github.api("branches/main/protection", missing=True) is not None:
        raise ReleaseError("이전 main 보호가 남아 승격 merge를 막을 수 있습니다")
    print("기본 브랜치와 preview → rc → main 보호 확인 완료")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true")
    args = parser.parse_args()
    configure(GitHub(os.environ.get("GITHUB_REPOSITORY", "NARUBROWN/spine")), args.apply)
