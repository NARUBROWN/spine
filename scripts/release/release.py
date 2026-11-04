#!/usr/bin/env python3
"""Preview, RC, Stable의 출처와 관찰 기간을 검사하고 승인된 계획만 발행합니다."""

import argparse
import base64
import datetime as dt
import json
import hashlib
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import zipfile


TAG = re.compile(r"v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-(preview|rc)\.([1-9]\d*))?\Z")
SHA = re.compile(r"[0-9a-f]{40}\Z")
WORKFLOWS = {"preview": "preview.yml", "rc": "rc.yml", "stable": "release.yml"}
CORE = "github.com/NARUBROWN/spine"
PLATFORMS = {CORE + suffix for suffix in (
    "-echo-platform", "-gorilla-platform", "-kafka-platform", "-rabbitmq-platform", "-validator-playground")}
MANAGED = {CORE, *PLATFORMS}


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
        raise ReleaseError("공개 시각이나 검증 기록 시각이 올바르지 않습니다") from exc


def require_age(published_at, hours, now, reason):
    age = (now - timestamp(published_at)).total_seconds() / 3600
    if age < 0:
        raise ReleaseError("공개 시각이 현재보다 뒤인 릴리스는 사용할 수 없습니다")
    if age < hours and not reason:
        raise ReleaseError(f"관찰 기간 부족: {age:.1f}시간 경과, {hours}시간 필요")


def validate_policy(policy):
    if policy.get("schema_version") != "spine.release-policy/v2":
        raise ReleaseError("지원하지 않는 릴리스 정책입니다")
    if parse_tag(policy["next_version"])[1] != "stable":
        raise ReleaseError("next_version에는 정식 버전을 지정해야 합니다")
    for name in ("preview_hours", "rc_hours", "stable_interval_hours"):
        if not isinstance(policy.get(name), int) or policy[name] <= 0:
            raise ReleaseError(f"{name}에는 양의 시간을 지정해야 합니다")
    kind = policy.get("kind")
    if kind not in ("core", "platform") or policy.get("module") not in MANAGED:
        raise ReleaseError("코어 또는 공식 플랫폼의 모듈 경로를 지정해야 합니다")
    if (kind == "core") != (policy["module"] == CORE):
        raise ReleaseError("릴리스 종류와 모듈 경로가 다릅니다")
    if set(policy.get("channels", {})) != {"preview", "rc", "stable"}:
        raise ReleaseError("각 채널의 의존성 조합을 지정해야 합니다")
    core_base = None
    for channel, selected in policy["channels"].items():
        base, actual_channel, _ = parse_tag(selected["core_version"])
        if actual_channel != channel or (core_base is not None and base != core_base):
            raise ReleaseError("플랫폼은 동일 대상 버전과 같은 채널의 코어를 참조해야 합니다")
        core_base = base
        modules = selected.get("modules", {})
        if kind == "core" and (base != policy["next_version"] or set(modules) != PLATFORMS):
            raise ReleaseError("코어는 다섯 플랫폼의 채널별 버전을 모두 고정해야 합니다")
        if kind == "platform" and modules:
            raise ReleaseError("플랫폼 릴리스는 코어 의존성만 지정합니다")
        for module, version in modules.items():
            if module not in PLATFORMS or parse_tag(version)[1] != channel:
                raise ReleaseError("통합 검증은 같은 채널의 공식 플랫폼 태그만 사용합니다")
    if policy.get("approval_environment") != "release-stable" or not policy.get("reviewer_login"):
        raise ReleaseError("정식 승인 환경과 지정 리뷰어가 필요합니다")
    if policy.get("branches") != {"preview": "preview", "rc": "rc", "stable": "main"}:
        raise ReleaseError("출시 브랜치는 preview → rc → main으로 지정해야 합니다")
    for module, sha in policy.get("ci_sources", {}).items():
        if module not in PLATFORMS or not SHA.fullmatch(sha):
            raise ReleaseError("소스 CI는 공식 플랫폼의 전체 커밋 SHA만 사용합니다")
    if kind == "core" and set(policy.get("ci_sources", {})) != PLATFORMS:
        raise ReleaseError("코어 소스 CI에는 다섯 플랫폼의 커밋이 필요합니다")
    if kind == "platform" and not SHA.fullmatch(policy.get("ci_core", "")):
        raise ReleaseError("플랫폼 소스 CI에는 코어의 전체 커밋 SHA가 필요합니다")


def components(policy, channel="preview"):
    return policy["channels"][channel]


def expected_modules(plan, scope):
    expected = {CORE: plan["components"]["core_version"]}
    if scope == "cohort":
        expected.update(plan["components"]["modules"])
    elif scope == "platform":
        expected[plan["module"]] = plan["version"]
    return expected


def selected_modules_match(plan, report, scope):
    selected = report.get("selected_modules", {})
    return isinstance(selected, dict) and all(selected.get(name) == version for name, version in expected_modules(plan, scope).items())


def normalized_file(path, content):
    # 승격 시 허용하는 차이는 같은 기본 버전의 채널 태그와 CI 출처뿐입니다.
    if path == ".github/release-policy.json":
        value = json.loads(content)
        for name in ("ci_sources", "ci_core", "post_release_examples"):
            value.pop(name, None)
        for selected in value["channels"].values():
            selected["core_version"] = parse_tag(selected["core_version"])[0]
            selected["modules"] = {name: parse_tag(version)[0] for name, version in selected.get("modules", {}).items()}
        return json.dumps(value, sort_keys=True, separators=(",", ":")).encode()
    if path.rsplit("/", 1)[-1] == "go.mod":
        names = "|".join(re.escape(name) for name in sorted(MANAGED))
        block = False
        lines = []
        for line in content.decode().splitlines(keepends=True):
            stripped = line.split("//", 1)[0].strip()
            if stripped == "require (":
                block = True
            elif stripped == ")":
                block = False
            pattern = r"^(\s*" + ("" if block else r"require\s+") + r"(?:" + names + r")\s+)(v[^\s]+)"
            line = re.sub(pattern, lambda m: m[1] + (parse_tag(m[2])[0] if TAG.fullmatch(m[2]) else m[2]), line)
            lines.append(line)
        return "".join(lines).encode()
    if path.rsplit("/", 1)[-1] == "go.sum":
        return "\n".join(line for line in content.decode().splitlines() if line.split(" ", 1)[0] not in MANAGED).encode()
    return content


def source_fingerprint(github, sha):
    tree = github.api(f"git/commits/{sha}")["tree"]["sha"]
    entries = github.api(f"git/trees/{tree}?recursive=1")
    if entries.get("truncated"):
        raise ReleaseError("전체 소스 트리를 확인하지 못했습니다")
    records = []
    for entry in entries["tree"]:
        if entry["type"] == "tree":
            continue
        path, digest = entry["path"], entry["sha"]
        if path == ".github/release-policy.json" or path.rsplit("/", 1)[-1] in ("go.mod", "go.sum"):
            blob = github.api("git/blobs/" + digest)
            content = base64.b64decode(blob["content"])
            digest = hashlib.sha256(normalized_file(path, content)).hexdigest()
        records.append((path, entry["mode"], entry["type"], digest))
    return hashlib.sha256(json.dumps(sorted(records), separators=(",", ":")).encode()).hexdigest()


def check_module(policy, channel, root):
    parsed = json.loads(run("go", "mod", "edit", "-json", cwd=root, env=dict(os.environ, GOWORK="off")))
    if parsed["Module"]["Path"] != policy["module"] or parsed.get("Replace"):
        raise ReleaseError("발행 모듈 경로가 다르거나 로컬 replace가 남아 있습니다")
    required = {item["Path"]: item["Version"] for item in (parsed.get("Require") or [])}
    if policy["kind"] == "platform" and required.get(CORE) != components(policy, channel)["core_version"]:
        raise ReleaseError("go.mod의 코어 버전이 발행 채널의 고정 버전과 다릅니다")
    if policy["kind"] == "core" and any(name in MANAGED for name in required):
        raise ReleaseError("코어 본체는 플랫폼 모듈에 의존하지 않습니다")
    if policy["kind"] == "core":
        expected = {CORE: components(policy, channel)["core_version"], **components(policy, channel)["modules"]}
        for name in ("test", "test/openapi", "test/platform-e2e", "cmd/demo", "cmd/reference", "examples/security-config"):
            child = json.loads(run("go", "mod", "edit", "-json", cwd=root / name, env=dict(os.environ, GOWORK="off")))
            for item in (child.get("Require") or []):
                if item["Path"] in MANAGED and item["Version"] != expected[item["Path"]]:
                    raise ReleaseError(f"{name}/go.mod가 발행 채널의 의존성과 다릅니다: {item['Path']}")


class GitHub:
    def __init__(self, repository):
        if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
            raise ReleaseError("저장소 이름이 올바르지 않습니다")
        self.repository = repository

    def api(self, path, *, missing=False, method="GET", data=None):
        endpoint = f"repos/{self.repository}" + ("/" + path if path else "")
        args = ["gh", "api", "--method", method, endpoint]
        if data is not None:
            args += ["--input", "-"]
        result = subprocess.run(args, input=json.dumps(data) if data is not None else None,
                                text=True, capture_output=True)
        if result.returncode:
            # 인증 또는 통신 오류를 '태그 없음'으로 해석하지 않습니다.
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

    def candidate_evidence(self, tag, prefix="release-manifest-"):
        parse_tag(tag)
        name = prefix + tag
        artifacts = []
        for page in range(1, 101):
            items = self.api(f"actions/artifacts?name={name}&per_page=100&page={page}")["artifacts"]
            artifacts.extend(item for item in items if item["name"] == name)
            if len(items) < 100:
                break
        else:
            raise ReleaseError("후보 검증 기록 목록을 모두 확인하지 못했습니다")
        if len(artifacts) != 1 or artifacts[0].get("expired", True):
            raise ReleaseError("태그에 대응하는 검증 artifact가 없거나 중복, 만료됐습니다. 새 Preview가 필요합니다")
        artifact = artifacts[0]
        workflow_run = self.api(f"actions/runs/{int(artifact['workflow_run']['id'])}")
        with tempfile.TemporaryDirectory(prefix="spine-release-manifest-") as directory:
            archive = Path(directory) / "manifest.zip"
            with archive.open("wb") as output:
                result = subprocess.run(["gh", "api", f"repos/{self.repository}/actions/artifacts/{int(artifact['id'])}/zip"],
                                        stdout=output, stderr=subprocess.PIPE)
            if result.returncode:
                raise ReleaseError("후보 검증 artifact를 다운로드하지 못했습니다: " + result.stderr.decode("utf-8", errors="replace").strip())
            if artifact.get("digest") != "sha256:" + hashlib.sha256(archive.read_bytes()).hexdigest():
                raise ReleaseError("후보 검증 artifact의 SHA-256이 일치하지 않습니다")
            try:
                with zipfile.ZipFile(archive) as package:
                    manifest = json.loads(package.read("release-manifest.json"))
            except (zipfile.BadZipFile, KeyError, ValueError) as exc:
                raise ReleaseError("후보 검증 artifact에 올바른 manifest가 없습니다") from exc
        return {"artifact": artifact, "workflow_run": workflow_run, "manifest": manifest}


def verify_source_candidate(github, source, expected_channel, policy, bootstrap=False):
    _, channel, _ = parse_tag(source)
    if channel != expected_channel:
        raise ReleaseError(f"이 단계의 출처는 {expected_channel} 태그여야 합니다")
    prefix = "core-candidate-" if bootstrap else "release-manifest-"
    evidence = github.candidate_evidence(source, prefix) if bootstrap else github.candidate_evidence(source)
    artifact, workflow_run, manifest = evidence["artifact"], evidence["workflow_run"], evidence["manifest"]
    sha = github.tag_sha(source)
    if not SHA.fullmatch(sha or ""):
        raise ReleaseError("후보 태그에 유효한 소스 SHA가 없습니다")
    tagged_policy = json.loads(base64.b64decode(github.api(f"contents/.github/release-policy.json?ref={sha}")["content"]))
    validate_policy(tagged_policy)
    scope = "core" if bootstrap else "cohort" if policy["kind"] == "core" else "platform"
    if (manifest.get("schema_version") != "spine.release-manifest/v1" or
            manifest.get("version") != source or manifest.get("channel") != channel or
            manifest.get("source_sha") != sha or not SHA.fullmatch(sha or "") or
            manifest.get("repository") != github.repository or
            manifest.get("kind") != policy["kind"] or manifest.get("module") != policy["module"] or
            manifest.get("branch") != policy["branches"][channel] or
            manifest.get("source_tree") != github.api(f"git/commits/{sha}")["tree"]["sha"] or
            manifest.get("source_fingerprint") != source_fingerprint(github, sha) or
            manifest.get("validation", {}).get("scope") != ("core" if bootstrap else "cohort" if policy["kind"] == "core" else "platform") or
            manifest.get("validation", {}).get("result") != "passed" or
            manifest.get("components") != components(tagged_policy, channel) or
            not selected_modules_match(manifest, manifest.get("validation", {}), scope) or
            manifest["validation"].get("source_sha") != sha or
            manifest["validation"].get("version") != source or
            manifest["validation"].get("components") != manifest.get("components")):
        raise ReleaseError("출처 태그와 검증 manifest가 일치하지 않습니다")
    artifact_run = artifact.get("workflow_run", {})
    if (workflow_run.get("conclusion") != "success" or workflow_run.get("status") != "completed" or
            workflow_run.get("event") != "workflow_dispatch" or
            workflow_run.get("head_branch") != policy["branches"][channel] or
            workflow_run.get("head_sha") != sha or
            str(workflow_run.get("id")) != manifest.get("workflow_run_id") or
            str(workflow_run.get("run_attempt")) != manifest.get("workflow_run_attempt") or
            artifact.get("name") != prefix + source or artifact.get("expired", True) or
            artifact_run.get("id") != workflow_run.get("id") or
            artifact_run.get("head_sha") != sha or artifact_run.get("head_branch") != policy["branches"][channel] or
            workflow_run.get("path", "").split("@")[0] != f".github/workflows/{'complete.yml' if policy['kind'] == 'core' and not bootstrap else WORKFLOWS[channel]}"):
        raise ReleaseError("출처 태그의 검증 workflow와 artifact가 일치하지 않거나 성공하지 않았습니다")
    return artifact, manifest


def complete_plan(github, policy, version, dependency_factory=GitHub):
    validate_policy(policy)
    base, channel, _ = parse_tag(version)
    if policy["kind"] != "core" or base != policy["next_version"]:
        raise ReleaseError("전체 조합 완료는 해당 버전의 코어에서만 실행합니다")
    artifact, candidate = verify_source_candidate(github, version, channel, policy, bootstrap=True)
    head = github.api("git/ref/heads/" + policy["branches"][channel])["object"]["sha"]
    source_policy = github.api(f"contents/.github/release-policy.json?ref={head}")
    if (os.environ.get("GITHUB_REF") != "refs/heads/" + policy["branches"][channel] or
            head != candidate["source_sha"] or candidate["components"] != components(policy, channel) or
            json.loads(base64.b64decode(source_policy["content"])) != policy):
        raise ReleaseError("코어 후보 발행 후 채널 HEAD 또는 조합이 바뀌었습니다")
    peers = {}
    for module, tag in components(policy, channel)["modules"].items():
        dependency = dependency_factory(module.removeprefix("github.com/"))
        sha = dependency.tag_sha(tag)
        if not SHA.fullmatch(sha or ""):
            raise ReleaseError("플랫폼 후보 태그가 아직 없습니다: " + module)
        raw_policy = dependency.api(f"contents/.github/release-policy.json?ref={sha}")
        peer_policy = json.loads(base64.b64decode(raw_policy["content"]))
        validate_policy(peer_policy)
        proof, manifest = verify_source_candidate(dependency, tag, channel, peer_policy)
        if (peer_policy["module"] != module or manifest["kind"] != "platform" or
                manifest["components"]["core_version"] != version):
            raise ReleaseError("플랫폼 검증이 다른 코어 후보를 사용했습니다: " + module)
        peers[module] = {"version": tag, "source_sha": sha, "artifact_id": proof["id"], "digest": proof["digest"]}
    return dict(candidate, bootstrap_evidence={"artifact_id": artifact["id"], "digest": artifact["digest"]},
                platform_evidence=peers, workflow_run_id=os.environ.get("GITHUB_RUN_ID", ""),
                workflow_run_attempt=os.environ.get("GITHUB_RUN_ATTEMPT", "1"))


def promotion(github, source_sha, upstream_sha, branch, upstream_branch, version=None):
    commit = github.api(f"git/commits/{source_sha}")
    upstream = github.api(f"git/commits/{upstream_sha}")
    parents = commit.get("parents", [])
    if len(parents) != 2:
        raise ReleaseError("채널 승격은 merge commit이어야 합니다")
    if parents[1]["sha"] != upstream_sha:
        comparison = github.api(f"compare/{upstream_sha}...{parents[1]['sha']}")
        if comparison["status"] not in ("ahead", "identical"):
            raise ReleaseError("승격 PR은 검증한 후보의 후속 커밋이어야 합니다")
    if source_fingerprint(github, source_sha) != source_fingerprint(github, upstream_sha):
        raise ReleaseError("승격에 채널 의존성 이외의 코드 변경이 있습니다. 새 Preview가 필요합니다")
    pulls = github.api(f"commits/{source_sha}/pulls?per_page=100")
    promotion_branch = f"promote/{'stable' if branch == 'main' else branch}/{version}"
    matches = [pr for pr in pulls if pr.get("merged_at") and pr.get("merge_commit_sha") == source_sha
               and pr["base"]["ref"] == branch and pr["head"]["ref"] in (upstream_branch, promotion_branch)
               and (pr["head"].get("repo") or {}).get("full_name") == github.repository]
    if not matches:
        raise ReleaseError(f"{upstream_branch} → {branch} 방향의 병합된 PR이 필요합니다")
    return {"number": matches[0]["number"], "head": upstream_branch, "base": branch,
            "merge_sha": source_sha, "head_sha": parents[1]["sha"], "promotion_branch": matches[0]["head"]["ref"]}


def approval_environment(github, policy):
    environment = github.api(f"environments/{policy['approval_environment']}")
    rules = environment.get("protection_rules", [])
    reviewers = [entry["reviewer"]["login"] for rule in rules if rule["type"] == "required_reviewers"
                 for entry in rule.get("reviewers", []) if entry["type"] == "User"]
    branches = environment.get("deployment_branch_policy") or {}
    if (policy["reviewer_login"] not in reviewers or environment.get("can_admins_bypass", True) or
            not branches.get("custom_branch_policies")):
        raise ReleaseError("정식 승인 환경에 지정 리뷰어, 우회 금지와 브랜치 제한이 필요합니다")
    branch_rules = github.api(f"environments/{policy['approval_environment']}/deployment-branch-policies")
    if not any(rule["name"] == "main" and rule.get("type", "branch") == "branch"
               for rule in branch_rules.get("branch_policies", [])):
        raise ReleaseError("정식 승인 환경은 main에서만 실행할 수 있어야 합니다")


def make_plan(github, policy, channel, version, source, reason="", now=None):
    validate_policy(policy)
    if github.repository != policy["module"].removeprefix("github.com/"):
        raise ReleaseError("정책에 지정한 모듈과 실행 저장소가 다릅니다")
    now = now or dt.datetime.now(dt.timezone.utc)
    base, actual_channel, _ = parse_tag(version)
    if base != policy["next_version"] or channel != actual_channel:
        raise ReleaseError("대상 버전과 채널이 정책의 next_version과 일치하지 않습니다")
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
    branch = policy["branches"][channel]
    source_sha = github.api(f"git/ref/heads/{branch}")["object"]["sha"]
    if not SHA.fullmatch(source_sha):
        raise ReleaseError("채널 브랜치가 유효한 커밋을 가리키지 않습니다")
    source_tag, upstream_sha, promotion_pr, source_evidence = "", "", None, None
    if channel == "preview":
        if not SHA.fullmatch(source) or source != source_sha:
            raise ReleaseError("Preview 출처에는 preview 브랜치 HEAD의 전체 커밋 SHA가 필요합니다")
    else:
        source_tag = source
        source_base, _, _ = parse_tag(source)
        if source_base != base:
            raise ReleaseError("다른 버전의 후보를 승격할 수 없습니다")
        expected = "preview" if channel == "rc" else "rc"
        artifact, manifest = verify_source_candidate(github, source, expected, policy)
        source_evidence = {"artifact_id": artifact["id"], "digest": artifact["digest"],
                           "workflow_run_id": manifest["workflow_run_id"], "created_at": artifact["created_at"]}
        upstream_sha = manifest["source_sha"]
        promotion_pr = promotion(github, source_sha, upstream_sha, branch, policy["branches"][expected], version)
        previous = manifest.get("components", {})
        if (parse_tag(previous.get("core_version", ""))[0] != parse_tag(components(policy, channel)["core_version"])[0] or
                {name: parse_tag(version)[0] for name, version in previous.get("modules", {}).items()} !=
                {name: parse_tag(version)[0] for name, version in components(policy, channel).get("modules", {}).items()}):
            raise ReleaseError("승격 시 기본 의존 버전을 변경할 수 없습니다. 새 Preview가 필요합니다")
        require_age(artifact["created_at"], policy[f"{expected}_hours"], now, reason)
    tree = github.api(f"git/commits/{source_sha}")["tree"]["sha"]
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
            "source_sha": source_sha, "source_tag": source_tag, "branch": branch, "source_tree": tree,
            "source_fingerprint": source_fingerprint(github, source_sha), "kind": policy["kind"], "module": policy["module"],
            "upstream_source_sha": upstream_sha, "promotion_pr": promotion_pr, "components": components(policy, channel),
            "source_evidence": source_evidence,
            "observation_override_reason": reason, "planned_at": now.isoformat(),
            "workflow_run_id": os.environ.get("GITHUB_RUN_ID", ""),
            "workflow_run_attempt": os.environ.get("GITHUB_RUN_ATTEMPT", "1"),
            "repository": github.repository}


def publish(github, policy, plan, verified_sha):
    if verified_sha != plan["source_sha"]:
        raise ReleaseError("검증한 커밋과 발행 대상이 다릅니다")
    if os.environ.get("GITHUB_REF") != "refs/heads/" + plan["branch"]:
        raise ReleaseError("릴리스 workflow의 실행 브랜치가 채널과 다릅니다")
    current = github.api("contents/.github/release-policy.json?ref=" + plan["branch"])
    if json.loads(base64.b64decode(current["content"])) != policy:
        raise ReleaseError("계획을 만든 뒤 채널 브랜치의 릴리스 정책이 바뀌었습니다")
    # 승인 대기 중 태그, 정책, 관찰 기간이 바뀌었는지 쓰기 직전에 다시 검사합니다.
    fresh = make_plan(github, policy, plan["channel"], plan["version"],
                      plan["source_tag"] or plan["source_sha"], plan["observation_override_reason"])
    for key in ("source_sha", "source_tree", "source_fingerprint", "kind", "module", "branch", "promotion_pr", "source_evidence", "channel", "version", "components"):
        if fresh[key] != plan[key]:
            raise ReleaseError("승인한 계획이 바뀌었습니다. 새 실행이 필요합니다")
    github.api("git/refs", method="POST", data={"ref": f"refs/tags/{plan['version']}", "sha": verified_sha})


def finish(github, plan, report, manifest_path="release-manifest.json"):
    if (report.get("source_sha") != plan["source_sha"] or report.get("version") != plan["version"] or
            report.get("result") != "passed" or report.get("components") != plan["components"] or
            report.get("scope") != ("cohort" if plan["kind"] == "core" else "platform") or
            not selected_modules_match(plan, report, report.get("scope")) or
            github.tag_sha(plan["version"]) != plan["source_sha"]):
        raise ReleaseError("공개 모듈 검증 결과가 발행 계획과 다릅니다")
    if plan["kind"] == "core":
        policy = json.loads(base64.b64decode(github.api(f"contents/.github/release-policy.json?ref={plan['source_sha']}")["content"]))
        fresh = complete_plan(github, policy, plan["version"])
        for key in ("source_sha", "source_fingerprint", "components", "bootstrap_evidence", "platform_evidence"):
            if plan.get(key) != fresh.get(key):
                raise ReleaseError("전체 조합 검증 중 후보 증거가 바뀌었습니다")
    manifest = dict(plan, validation=report)
    path = Path(manifest_path).resolve()
    path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    # 후보는 태그와 Actions artifact로 검증 기록을 남기고 정식 버전만 GitHub Release를 공개합니다.
    if plan["channel"] != "stable":
        return
    with tempfile.TemporaryDirectory(prefix="spine-release-publish-") as directory:
        notes = Path(directory) / "notes.md"
        notes.write_text(f"커밋: `{plan['source_sha']}`\n\n"
                         f"채널 브랜치: `{plan['branch']}`\n\n"
                         f"이전 단계: `{plan['source_tag'] or 'preview'}`\n\n"
                         f"전환 안내: https://github.com/{github.repository}/blob/{plan['source_sha']}/docs/migration/{parse_tag(plan['version'])[0]}.md\n\n"
                         f"관찰 기간 단축 사유: {plan['observation_override_reason'] or '없음'}\n\n"
                         "검증한 코어와 플랫폼 버전, 실행 증거는 첨부 manifest를 확인하세요. 예제는 정식 발행 후 갱신합니다.\n")
        args = ["gh", "release", "create", plan["version"], str(path), "--repo", github.repository,
                "--verify-tag", "--title", plan["version"], "--notes-file", str(notes), "--latest"]
        run(*args)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["plan", "tag", "record-core", "complete-plan", "finish", "validate-policy", "check-environment", "check-module"])
    parser.add_argument("--channel", choices=["preview", "rc", "stable"], default="preview")
    parser.add_argument("--root", default=".")
    parser.add_argument("--version")
    parser.add_argument("--policy", default=".github/release-policy.json")
    parser.add_argument("--plan", default="release-plan.json")
    parser.add_argument("--report", default="release-validation.json")
    parser.add_argument("--manifest", default="release-manifest.json")
    args = parser.parse_args()
    policy = json.loads(Path(args.policy).read_text())
    validate_policy(policy)
    if args.command == "validate-policy":
        root = Path(__file__).resolve().parents[2]
        if policy["kind"] == "core":
            bootstrap = (root / "internal/bootstrap/bootstrap.go").read_text()
            if f'spineVersion = "{policy["next_version"]}"' not in bootstrap:
                raise ReleaseError("런타임 개발 기준 버전과 next_version이 다릅니다")
        if not (root / "docs/migration" / (policy["next_version"]+".md")).is_file():
            raise ReleaseError("대상 버전의 전환 안내가 필요합니다")
        names = ("README.md", "README.en.md", "README.ja.md", "README.zh-Hans.md", "README.zh-Hant.md") if policy["kind"] == "core" else ("README.md",)
        for name in names:
            if f'`{policy["next_version"]}`' not in (root / name).read_text():
                raise ReleaseError(f"{name}의 대상 버전을 확인하세요")
        print("릴리스 정책 형식 확인 완료")
        return
    if args.command == "check-module":
        check_module(policy, args.channel, Path(args.root))
        return
    github = GitHub(os.environ["GITHUB_REPOSITORY"])
    if args.command == "check-environment":
        approval_environment(github, policy)
        print("정식 릴리스 승인 환경 확인 완료")
        return
    if args.command == "complete-plan":
        plan = complete_plan(github, policy, args.version)
        Path(args.plan).write_text(json.dumps(plan, ensure_ascii=False, indent=2) + "\n")
        if output := os.environ.get("GITHUB_OUTPUT"):
            with open(output, "a") as stream:
                stream.write(f"sha={plan['source_sha']}\nversion={plan['version']}\nchannel={plan['channel']}\n")
        return
    if args.command == "plan":
        channel = os.environ["RELEASE_CHANNEL"]
        if os.environ.get("GITHUB_REF") != "refs/heads/" + policy["branches"][channel]:
            raise ReleaseError("채널에 맞는 preview, rc, main 브랜치에서 workflow를 실행해야 합니다")
        source = os.environ["RELEASE_SOURCE"]
        version = parse_tag(source)[0] if channel == "stable" else os.environ["RELEASE_VERSION"]
        check_module(policy, channel, Path(args.root))
        if policy["kind"] == "core" and version != components(policy, channel)["core_version"]:
            raise ReleaseError("코어 후보 태그와 채널 조합의 코어 태그가 다릅니다")
        plan = make_plan(github, policy, channel, version, source, os.environ.get("RELEASE_OVERRIDE_REASON", ""))
        Path(args.plan).write_text(json.dumps(plan, ensure_ascii=False, indent=2) + "\n")
        if output := os.environ.get("GITHUB_OUTPUT"):
            with open(output, "a") as stream:
                stream.write(f"sha={plan['source_sha']}\nversion={plan['version']}\nkind={plan['kind']}\n")
        if summary := os.environ.get("GITHUB_STEP_SUMMARY"):
            with open(summary, "a") as stream:
                stream.write(f"출시 계획: `{plan['version']}` / `{plan['source_sha']}`\n\n"
                             f"관찰 기간 단축 사유: {plan['observation_override_reason'] or '없음'}\n")
    else:
        plan = json.loads(Path(args.plan).read_text())
        if args.command == "tag":
            publish(github, policy, plan, os.environ["VERIFIED_SHA"])
        elif args.command == "record-core":
            report = json.loads(Path(args.report).read_text())
            if (plan["kind"] != "core" or report.get("scope") != "core" or report.get("result") != "passed" or
                    report.get("components") != plan["components"] or report.get("source_sha") != plan["source_sha"] or
                    report.get("version") != plan["version"] or github.tag_sha(plan["version"]) != plan["source_sha"]):
                raise ReleaseError("코어 후보의 공개 검증 결과가 계획과 다릅니다")
            if not selected_modules_match(plan, report, "core"):
                raise ReleaseError("공개 소비자가 계획한 코어 버전을 선택하지 않았습니다")
            Path(args.manifest).write_text(json.dumps(dict(plan, validation=report), ensure_ascii=False, indent=2)+"\n")
        else:
            finish(github, plan, json.loads(Path(args.report).read_text()), args.manifest)


if __name__ == "__main__":
    try:
        main()
    except (ReleaseError, KeyError, ValueError) as exc:
        print(f"릴리스 중단: {exc}", file=sys.stderr)
        sys.exit(1)
