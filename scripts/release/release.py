#!/usr/bin/env python3
"""Preview, RC, Stable의 출처와 관찰 기간을 검사하고 승인된 계획만 발행합니다."""

import argparse
import base64
import datetime as dt
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile


TAG = re.compile(r"v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-(preview|rc)\.([1-9]\d*))?\Z")
SHA = re.compile(r"[0-9a-f]{40}\Z")
WORKFLOWS = {"preview": "preview.yml", "rc": "rc.yml", "stable": "release.yml"}


class ReleaseError(Exception):
    pass


def run(*args, **kwargs):
    result = subprocess.run(args, text=True, capture_output=True, **kwargs)
    if result.returncode:
        raise ReleaseError(f"명령 실패: {args[0]} {args[1]}\n{result.stderr.strip()}")
    return result.stdout.strip()


def parse_tag(value):
    match = TAG.fullmatch(value)
    if not match:
        raise ReleaseError("버전은 vX.Y.Z-preview.N, vX.Y.Z-rc.N 또는 vX.Y.Z 형식이어야 합니다")
    major, minor, patch, channel, number = match.groups()
    return f"v{major}.{minor}.{patch}", channel or "stable", int(number or 0)


def timestamp(value):
    try:
        return dt.datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(dt.timezone.utc)
    except (AttributeError, ValueError) as exc:
        raise ReleaseError("공개 시각이 없는 릴리스는 승격할 수 없습니다") from exc


def require_age(published_at, hours, now, reason):
    age = (now - timestamp(published_at)).total_seconds() / 3600
    if age < 0:
        raise ReleaseError("공개 시각이 현재보다 뒤인 릴리스는 사용할 수 없습니다")
    if age < hours and not reason:
        raise ReleaseError(f"관찰 기간 부족: {age:.1f}시간 경과, {hours}시간 필요")


def validate_policy(policy):
    if policy.get("schema_version") != "spine.release-policy/v1":
        raise ReleaseError("지원하지 않는 릴리스 정책입니다")
    if parse_tag(policy["next_version"])[1] != "stable":
        raise ReleaseError("next_version에는 정식 버전을 지정해야 합니다")
    for name in ("preview_hours", "rc_hours", "stable_interval_hours"):
        if not isinstance(policy.get(name), int) or policy[name] <= 0:
            raise ReleaseError(f"{name}에는 양의 시간을 지정해야 합니다")
    if set(policy.get("examples", {})) != {"chat", "msa"} or not policy.get("modules"):
        raise ReleaseError("채팅·MSA 예제와 플랫폼 버전 조합을 지정해야 합니다")
    if policy.get("approval_environment") != "release-stable" or not policy.get("reviewer_login"):
        raise ReleaseError("정식 승인 환경과 지정 리뷰어가 필요합니다")
    for version in [policy["cli_version"], *policy["modules"].values()]:
        if not re.fullmatch(r"v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?", version):
            raise ReleaseError("의존성은 변경 가능한 브랜치 대신 버전으로 고정해야 합니다")
    for example in policy["examples"].values():
        if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", example["repository"]) or not SHA.fullmatch(example["ref"]):
            raise ReleaseError("예제 저장소와 전체 커밋 SHA를 고정해야 합니다")


class GitHub:
    def __init__(self, repository):
        if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
            raise ReleaseError("저장소 이름이 올바르지 않습니다")
        self.repository = repository

    def api(self, path, *, missing=False, method="GET", data=None):
        args = ["gh", "api", "--method", method, f"repos/{self.repository}/{path}"]
        if data is not None:
            args += ["--input", "-"]
        result = subprocess.run(args, input=json.dumps(data) if data is not None else None,
                                text=True, capture_output=True)
        if result.returncode:
            # 인증·통신 오류를 '태그 없음'으로 해석하지 않습니다.
            try:
                status = json.loads(result.stdout).get("status")
            except (ValueError, AttributeError):
                status = None
            if missing and str(status) == "404":
                return None
            raise ReleaseError(f"GitHub API 실패 ({path}): {result.stderr.strip()}")
        return json.loads(result.stdout) if result.stdout.strip() else None

    def releases(self):
        result = []
        for page in range(1, 101):
            items = self.api(f"releases?per_page=100&page={page}")
            result.extend(items)
            if len(items) < 100:
                return result
        raise ReleaseError("릴리스 목록이 너무 많아 전체 확인을 마치지 못했습니다")

    def tag_sha(self, tag):
        parse_tag(tag)
        ref = self.api(f"git/ref/tags/{tag}", missing=True)
        if ref is None:
            return None
        obj = ref["object"]
        for _ in range(5):
            if obj["type"] == "commit":
                return obj["sha"]
            if obj["type"] != "tag":
                break
            obj = self.api(f"git/tags/{obj['sha']}")["object"]
        raise ReleaseError("태그가 단일 커밋을 가리키지 않습니다")

    def manifest(self, tag):
        with tempfile.TemporaryDirectory(prefix="spine-release-manifest-") as directory:
            run("gh", "release", "download", tag, "--repo", self.repository,
                "--pattern", "release-manifest.json", "--dir", directory)
            return json.loads((Path(directory) / "release-manifest.json").read_text())


def verify_source_release(github, source, expected_channel):
    _, channel, _ = parse_tag(source)
    if channel != expected_channel:
        raise ReleaseError(f"이 단계의 출처는 {expected_channel} 태그여야 합니다")
    release = github.api(f"releases/tags/{source}")
    if release.get("draft") or not release.get("prerelease"):
        raise ReleaseError("공개된 사전 릴리스만 승격할 수 있습니다")
    manifest = github.manifest(source)
    sha = github.tag_sha(source)
    if (manifest.get("schema_version") != "spine.release-manifest/v1" or
            manifest.get("version") != source or manifest.get("channel") != channel or
            manifest.get("source_sha") != sha or not SHA.fullmatch(sha or "") or
            manifest.get("validation", {}).get("result") != "passed" or
            manifest["validation"].get("source_sha") != sha or
            manifest["validation"].get("version") != source or
            manifest["validation"].get("components") != manifest.get("components")):
        raise ReleaseError("출처 태그와 릴리스 manifest가 일치하지 않습니다")
    workflow_run = github.api(f"actions/runs/{int(manifest['workflow_run_id'])}")
    if (workflow_run.get("conclusion") != "success" or workflow_run.get("status") != "completed" or
            workflow_run.get("event") != "workflow_dispatch" or
            workflow_run.get("path", "").split("@")[0] != f".github/workflows/{WORKFLOWS[channel]}"):
        raise ReleaseError("출처 릴리스의 발행 workflow가 성공하지 않았습니다")
    return release, manifest


def approval_environment(github, policy):
    environment = github.api(f"environments/{policy['approval_environment']}")
    rules = environment.get("protection_rules", [])
    reviewers = [entry["reviewer"]["login"] for rule in rules if rule["type"] == "required_reviewers"
                 for entry in rule.get("reviewers", []) if entry["type"] == "User"]
    branches = environment.get("deployment_branch_policy") or {}
    if (policy["reviewer_login"] not in reviewers or environment.get("can_admins_bypass", True) or
            not branches.get("custom_branch_policies")):
        raise ReleaseError("정식 승인 환경에 지정 리뷰어·우회 금지·브랜치 제한이 필요합니다")
    branch_rules = github.api(f"environments/{policy['approval_environment']}/deployment-branch-policies")
    if not any(rule["name"] == "main" and rule.get("type", "branch") == "branch"
               for rule in branch_rules.get("branch_policies", [])):
        raise ReleaseError("정식 승인 환경은 main에서만 실행할 수 있어야 합니다")


def make_plan(github, policy, channel, version, source, reason="", now=None):
    validate_policy(policy)
    now = now or dt.datetime.now(dt.timezone.utc)
    base, actual_channel, _ = parse_tag(version)
    if base != policy["next_version"] or channel != actual_channel:
        raise ReleaseError("대상 버전·채널이 정책의 next_version과 일치하지 않습니다")
    reason = reason.strip()
    if reason and len(reason) < 20:
        raise ReleaseError("긴급 단축 사유를 20자 이상 기록해야 합니다")
    if github.tag_sha(version) is not None:
        raise ReleaseError("이미 존재하는 태그는 이동하거나 재사용하지 않습니다")
    if channel != "stable" and github.tag_sha(base) is not None:
        raise ReleaseError("이미 정식 발행한 버전에는 사전 릴리스를 추가할 수 없습니다")
    if channel != "stable":
        siblings = github.api(f"git/matching-refs/tags/{base}-{channel}.")
        numbers = [parse_tag(item["ref"].removeprefix("refs/tags/"))[2] for item in siblings
                   if TAG.fullmatch(item["ref"].removeprefix("refs/tags/"))]
        if numbers and parse_tag(version)[2] <= max(numbers):
            raise ReleaseError("같은 채널의 새 번호는 기존 번호보다 커야 합니다")
    source_tag = ""
    if channel == "preview":
        if not SHA.fullmatch(source):
            raise ReleaseError("Preview 출처에는 main에 포함된 전체 커밋 SHA가 필요합니다")
        source_sha = source
    else:
        source_tag = source
        source_base, _, _ = parse_tag(source)
        if source_base != base:
            raise ReleaseError("다른 버전의 후보를 승격할 수 없습니다")
        expected = "preview" if channel == "rc" else "rc"
        release, manifest = verify_source_release(github, source, expected)
        source_sha = manifest["source_sha"]
        # 코드와 호환 버전 조합은 승인된 후보에서 그대로 이어받습니다.
        if manifest.get("components") != components(policy):
            raise ReleaseError("후보 검증 뒤 호환 버전 조합이 바뀌었습니다. 새 Preview가 필요합니다")
        require_age(release["published_at"], policy[f"{expected}_hours"], now, reason)
    comparison = github.api(f"compare/{source_sha}...main")
    if comparison.get("status") not in ("ahead", "identical"):
        raise ReleaseError("출시 커밋이 main의 이력에 포함되지 않습니다")
    source_policy = github.api(f"contents/.github/release-policy.json?ref={source_sha}")
    if json.loads(base64.b64decode(source_policy["content"])) != policy:
        raise ReleaseError("후보 커밋의 릴리스 정책이 현재 계획과 다릅니다. 대상 SHA를 확인하세요")
    if channel == "stable":
        approval_environment(github, policy)
        stable = [r for r in github.releases() if not r.get("draft") and not r.get("prerelease")
                  and TAG.fullmatch(r["tag_name"]) and parse_tag(r["tag_name"])[1] == "stable"]
        if stable:
            highest = max(tuple(map(int, parse_tag(r["tag_name"])[0][1:].split("."))) for r in stable)
            if tuple(map(int, base[1:].split("."))) <= highest:
                raise ReleaseError("기존 정식 버전보다 높은 버전만 발행할 수 있습니다")
            last = max(stable, key=lambda r: timestamp(r["published_at"]))
            require_age(last["published_at"], policy["stable_interval_hours"], now, reason)
    return {"schema_version": "spine.release-manifest/v1", "channel": channel, "version": version,
            "source_sha": source_sha, "source_tag": source_tag, "components": components(policy),
            "observation_override_reason": reason, "planned_at": now.isoformat(),
            "workflow_run_id": os.environ.get("GITHUB_RUN_ID", ""),
            "repository": github.repository}


def components(policy):
    return {"cli_version": policy["cli_version"], "modules": policy["modules"], "examples": policy["examples"]}


def publish(github, policy, plan, verified_sha):
    if verified_sha != plan["source_sha"]:
        raise ReleaseError("검증한 커밋과 발행 대상이 다릅니다")
    if os.environ.get("GITHUB_REF") != "refs/heads/main":
        raise ReleaseError("릴리스 workflow는 main에서만 실행할 수 있습니다")
    current = github.api("contents/.github/release-policy.json?ref=main")
    if json.loads(base64.b64decode(current["content"])) != policy:
        raise ReleaseError("계획을 만든 뒤 main의 릴리스 정책이 바뀌었습니다")
    # 승인 대기 중 태그·정책·관찰 기간이 바뀌었는지 쓰기 직전에 다시 검사합니다.
    fresh = make_plan(github, policy, plan["channel"], plan["version"],
                      plan["source_tag"] or plan["source_sha"], plan["observation_override_reason"])
    for key in ("source_sha", "channel", "version", "components"):
        if fresh[key] != plan[key]:
            raise ReleaseError("승인한 계획이 바뀌었습니다. 새 실행이 필요합니다")
    github.api("git/refs", method="POST", data={"ref": f"refs/tags/{plan['version']}", "sha": verified_sha})


def finish(github, plan, report):
    if (report.get("source_sha") != plan["source_sha"] or report.get("version") != plan["version"] or
            report.get("result") != "passed" or report.get("components") != plan["components"] or
            github.tag_sha(plan["version"]) != plan["source_sha"]):
        raise ReleaseError("공개 모듈 검증 결과가 발행 계획과 다릅니다")
    manifest = dict(plan, validation=report)
    with tempfile.TemporaryDirectory(prefix="spine-release-publish-") as directory:
        path = Path(directory) / "release-manifest.json"
        path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
        notes = Path(directory) / "notes.md"
        notes.write_text(f"커밋: `{plan['source_sha']}`\n\n"
                         f"이전 단계: `{plan['source_tag'] or 'main'}`\n\n"
                         f"전환 안내: https://github.com/{github.repository}/blob/{plan['source_sha']}/docs/migration/{parse_tag(plan['version'])[0]}.md\n\n"
                         f"관찰 기간 단축 사유: {plan['observation_override_reason'] or '없음'}\n\n"
                         "검증한 CLI·플랫폼·예제 버전과 실행 증거는 첨부 manifest를 확인하세요.\n")
        args = ["gh", "release", "create", plan["version"], str(path), "--repo", github.repository,
                "--verify-tag", "--title", plan["version"], "--notes-file", str(notes)]
        args += ["--latest"] if plan["channel"] == "stable" else ["--prerelease", "--latest=false"]
        run(*args)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["plan", "tag", "finish", "validate-policy", "check-environment"])
    parser.add_argument("--policy", default=".github/release-policy.json")
    parser.add_argument("--plan", default="release-plan.json")
    parser.add_argument("--report", default="release-validation.json")
    args = parser.parse_args()
    policy = json.loads(Path(args.policy).read_text())
    validate_policy(policy)
    if args.command == "validate-policy":
        root = Path(__file__).resolve().parents[2]
        bootstrap = (root / "internal/bootstrap/bootstrap.go").read_text()
        if f'spineVersion = "{policy["next_version"]}"' not in bootstrap:
            raise ReleaseError("런타임 개발 기준 버전과 next_version이 다릅니다")
        if not (root / "docs/migration" / (policy["next_version"]+".md")).is_file():
            raise ReleaseError("대상 버전의 전환 안내가 필요합니다")
        for name in ("README.md", "README.en.md", "README.ja.md", "README.zh-Hans.md", "README.zh-Hant.md"):
            if f'`{policy["next_version"]}`' not in (root / name).read_text():
                raise ReleaseError(f"{name}의 대상 버전을 확인하세요")
        print("릴리스 정책 형식 확인 완료")
        return
    github = GitHub(os.environ["GITHUB_REPOSITORY"])
    if args.command == "check-environment":
        approval_environment(github, policy)
        print("정식 릴리스 승인 환경 확인 완료")
        return
    if args.command == "plan":
        if os.environ.get("GITHUB_REF") != "refs/heads/main":
            raise ReleaseError("main에서 workflow를 실행해야 합니다")
        channel = os.environ["RELEASE_CHANNEL"]
        source = os.environ["RELEASE_SOURCE"]
        version = parse_tag(source)[0] if channel == "stable" else os.environ["RELEASE_VERSION"]
        plan = make_plan(github, policy, channel, version, source, os.environ.get("RELEASE_OVERRIDE_REASON", ""))
        Path(args.plan).write_text(json.dumps(plan, ensure_ascii=False, indent=2) + "\n")
        if output := os.environ.get("GITHUB_OUTPUT"):
            with open(output, "a") as stream:
                stream.write(f"sha={plan['source_sha']}\nversion={plan['version']}\n")
        if summary := os.environ.get("GITHUB_STEP_SUMMARY"):
            with open(summary, "a") as stream:
                stream.write(f"출시 계획: `{plan['version']}` / `{plan['source_sha']}`\n\n"
                             f"관찰 기간 단축 사유: {plan['observation_override_reason'] or '없음'}\n")
    else:
        plan = json.loads(Path(args.plan).read_text())
        if args.command == "tag":
            publish(github, policy, plan, os.environ["VERIFIED_SHA"])
        else:
            finish(github, plan, json.loads(Path(args.report).read_text()))


if __name__ == "__main__":
    try:
        main()
    except (ReleaseError, KeyError, ValueError) as exc:
        print(f"릴리스 중단: {exc}", file=sys.stderr)
        sys.exit(1)
