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
        raise ReleaseError(f"Command failed: {args[0]} {args[1]}\n{result.stderr.strip()}")
    return result.stdout.strip()


def parse_tag(value):
    match = TAG.fullmatch(value)
    if not match:
        raise ReleaseError("Version must use the vX.Y.Z-preview.N, vX.Y.Z-rc.N or vX.Y.Z format")
    major, minor, patch, channel, number = match.groups()
    return f"v{major}.{minor}.{patch}", channel or "stable", int(number or 0)


def timestamp(value):
    try:
        return dt.datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(dt.timezone.utc)
    except (AttributeError, ValueError) as exc:
        raise ReleaseError("Invalid publication or verification timestamp") from exc


def require_age(published_at, hours, now, reason):
    age = (now - timestamp(published_at)).total_seconds() / 3600
    if age < 0:
        raise ReleaseError("A release with a publication time in the future cannot be used")
    if age < hours and not reason:
        raise ReleaseError(f"Insufficient observation period: {age:.1f} hours elapsed, {hours} hours required")


def validate_policy(policy):
    if policy.get("schema_version") != "spine.release-policy/v2":
        raise ReleaseError("Unsupported release policy")
    if parse_tag(policy["next_version"])[1] != "stable":
        raise ReleaseError("next_version must specify a stable version")
    for name in ("preview_hours", "rc_hours", "stable_interval_hours"):
        if not isinstance(policy.get(name), int) or policy[name] <= 0:
            raise ReleaseError(f"{name} must specify a positive number of hours")
    kind = policy.get("kind")
    if kind not in ("core", "platform") or policy.get("module") not in MANAGED:
        raise ReleaseError("Specify the module path of the core or an official platform")
    if (kind == "core") != (policy["module"] == CORE):
        raise ReleaseError("Release kind does not match the module path")
    if set(policy.get("channels", {})) != {"preview", "rc", "stable"}:
        raise ReleaseError("Specify component versions for each channel")
    core_base = None
    for channel, selected in policy["channels"].items():
        base, actual_channel, _ = parse_tag(selected["core_version"])
        if actual_channel != channel or (core_base is not None and base != core_base):
            raise ReleaseError("Platforms must reference the same target core version and channel")
        core_base = base
        modules = selected.get("modules", {})
        if kind == "core" and (base != policy["next_version"] or set(modules) != PLATFORMS):
            raise ReleaseError("The core must pin all five platform versions for each channel")
        if kind == "platform" and modules:
            raise ReleaseError("Platform releases specify only the core dependency")
        for module, version in modules.items():
            if module not in PLATFORMS or parse_tag(version)[1] != channel:
                raise ReleaseError("Integration verification requires official platform tags from the same channel")
    if policy.get("approval_environment") != "release-stable" or not policy.get("reviewer_login"):
        raise ReleaseError("A stable release approval environment and designated reviewers are required")
    if policy.get("branches") != {"preview": "preview", "rc": "rc", "stable": "main"}:
        raise ReleaseError("Release branches must follow preview → rc → main")
    for module, sha in policy.get("ci_sources", {}).items():
        if module not in PLATFORMS or not SHA.fullmatch(sha):
            raise ReleaseError("Source CI requires full commit SHAs for official platforms")
    if kind == "core" and set(policy.get("ci_sources", {})) != PLATFORMS:
        raise ReleaseError("Core source CI requires commits for all five platforms")
    if kind == "platform" and not SHA.fullmatch(policy.get("ci_core", "")):
        raise ReleaseError("Platform source CI requires the full core commit SHA")


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
        raise ReleaseError("Failed to inspect the complete source tree")
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
        raise ReleaseError("Published module path does not match or local replace directives remain")
    required = {item["Path"]: item["Version"] for item in (parsed.get("Require") or [])}
    if policy["kind"] == "platform" and required.get(CORE) != components(policy, channel)["core_version"]:
        raise ReleaseError("The core version in go.mod does not match the pinned version for the publication channel")
    if policy["kind"] == "core" and any(name in MANAGED for name in required):
        raise ReleaseError("The core module must not depend on platform modules")
    if policy["kind"] == "core":
        expected = {CORE: components(policy, channel)["core_version"], **components(policy, channel)["modules"]}
        for name in ("test", "test/openapi", "test/platform-e2e", "cmd/demo", "cmd/reference", "examples/security-config"):
            child = json.loads(run("go", "mod", "edit", "-json", cwd=root / name, env=dict(os.environ, GOWORK="off")))
            for item in (child.get("Require") or []):
                if item["Path"] in MANAGED and item["Version"] != expected[item["Path"]]:
                    raise ReleaseError(f"{name}/go.mod does not match publication channel dependencies: {item['Path']}")


class GitHub:
    def __init__(self, repository):
        if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
            raise ReleaseError("Invalid repository name")
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
            raise ReleaseError(f"GitHub API request failed ({path}): {result.stderr.strip()}")
        return json.loads(result.stdout) if result.stdout.strip() else None

    def releases(self):
        result = []
        for page in range(1, 101):
            items = self.api(f"releases?per_page=100&page={page}")
            result.extend(items)
            if len(items) < 100:
                return result
        raise ReleaseError("The release list is too large to inspect completely")

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
        raise ReleaseError("The tag does not resolve to a single commit")

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
            raise ReleaseError("Failed to inspect all candidate verification records")
        if len(artifacts) != 1 or artifacts[0].get("expired", True):
            raise ReleaseError("The verification artifact for the tag is missing, duplicated or expired. Publish a new Preview.")
        artifact = artifacts[0]
        workflow_run = self.api(f"actions/runs/{int(artifact['workflow_run']['id'])}")
        with tempfile.TemporaryDirectory(prefix="spine-release-manifest-") as directory:
            archive = Path(directory) / "manifest.zip"
            with archive.open("wb") as output:
                result = subprocess.run(["gh", "api", f"repos/{self.repository}/actions/artifacts/{int(artifact['id'])}/zip"],
                                        stdout=output, stderr=subprocess.PIPE)
            if result.returncode:
                raise ReleaseError("Failed to download candidate verification artifact: " + result.stderr.decode("utf-8", errors="replace").strip())
            if artifact.get("digest") != "sha256:" + hashlib.sha256(archive.read_bytes()).hexdigest():
                raise ReleaseError("Candidate verification artifact SHA-256 does not match")
            try:
                with zipfile.ZipFile(archive) as package:
                    manifest = json.loads(package.read("release-manifest.json"))
            except (zipfile.BadZipFile, KeyError, ValueError) as exc:
                raise ReleaseError("Candidate verification artifact has no valid manifest") from exc
        return {"artifact": artifact, "workflow_run": workflow_run, "manifest": manifest}


def verify_source_candidate(github, source, expected_channel, policy, bootstrap=False):
    _, channel, _ = parse_tag(source)
    if channel != expected_channel:
        raise ReleaseError(f"This stage requires a {expected_channel} source tag")
    prefix = "core-candidate-" if bootstrap else "release-manifest-"
    evidence = github.candidate_evidence(source, prefix) if bootstrap else github.candidate_evidence(source)
    artifact, workflow_run, manifest = evidence["artifact"], evidence["workflow_run"], evidence["manifest"]
    sha = github.tag_sha(source)
    if not SHA.fullmatch(sha or ""):
        raise ReleaseError("The candidate tag has no valid source SHA")
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
        raise ReleaseError("Source tag does not match the verification manifest")
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
        raise ReleaseError("Source tag verification workflow and artifact do not match or verification did not succeed")
    return artifact, manifest


def complete_plan(github, policy, version, dependency_factory=GitHub):
    validate_policy(policy)
    base, channel, _ = parse_tag(version)
    if policy["kind"] != "core" or base != policy["next_version"]:
        raise ReleaseError("Component verification completion must run from the core repository for that version")
    artifact, candidate = verify_source_candidate(github, version, channel, policy, bootstrap=True)
    head = github.api("git/ref/heads/" + policy["branches"][channel])["object"]["sha"]
    source_policy = github.api(f"contents/.github/release-policy.json?ref={head}")
    if (os.environ.get("GITHUB_REF") != "refs/heads/" + policy["branches"][channel] or
            head != candidate["source_sha"] or candidate["components"] != components(policy, channel) or
            json.loads(base64.b64decode(source_policy["content"])) != policy):
        raise ReleaseError("Channel HEAD or component versions changed after the core candidate was published")
    peers = {}
    for module, tag in components(policy, channel)["modules"].items():
        dependency = dependency_factory(module.removeprefix("github.com/"))
        sha = dependency.tag_sha(tag)
        if not SHA.fullmatch(sha or ""):
            raise ReleaseError("Platform candidate tag is not available yet: " + module)
        raw_policy = dependency.api(f"contents/.github/release-policy.json?ref={sha}")
        peer_policy = json.loads(base64.b64decode(raw_policy["content"]))
        validate_policy(peer_policy)
        proof, manifest = verify_source_candidate(dependency, tag, channel, peer_policy)
        if (peer_policy["module"] != module or manifest["kind"] != "platform" or
                manifest["components"]["core_version"] != version):
            raise ReleaseError("Platform verification used a different core candidate: " + module)
        peers[module] = {"version": tag, "source_sha": sha, "artifact_id": proof["id"], "digest": proof["digest"]}
    return dict(candidate, bootstrap_evidence={"artifact_id": artifact["id"], "digest": artifact["digest"]},
                platform_evidence=peers, workflow_run_id=os.environ.get("GITHUB_RUN_ID", ""),
                workflow_run_attempt=os.environ.get("GITHUB_RUN_ATTEMPT", "1"))


def promotion(github, source_sha, upstream_sha, branch, upstream_branch, version=None):
    commit = github.api(f"git/commits/{source_sha}")
    upstream = github.api(f"git/commits/{upstream_sha}")
    parents = commit.get("parents", [])
    if len(parents) != 2:
        raise ReleaseError("Channel promotion requires a merge commit")
    if parents[1]["sha"] != upstream_sha:
        comparison = github.api(f"compare/{upstream_sha}...{parents[1]['sha']}")
        if comparison["status"] not in ("ahead", "identical"):
            raise ReleaseError("The promotion PR must descend from the verified candidate")
    if source_fingerprint(github, source_sha) != source_fingerprint(github, upstream_sha):
        raise ReleaseError("Promotion includes code changes beyond channel dependencies. Publish a new Preview.")
    pulls = github.api(f"commits/{source_sha}/pulls?per_page=100")
    promotion_branch = f"promote/{'stable' if branch == 'main' else branch}/{version}"
    matches = [pr for pr in pulls if pr.get("merged_at") and pr.get("merge_commit_sha") == source_sha
               and pr["base"]["ref"] == branch and pr["head"]["ref"] in (upstream_branch, promotion_branch)
               and (pr["head"].get("repo") or {}).get("full_name") == github.repository]
    if not matches:
        raise ReleaseError(f"{upstream_branch} → {branch} requires a merged promotion PR")
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
        raise ReleaseError("The stable approval environment requires designated reviewers, no bypass and branch restrictions")
    branch_rules = github.api(f"environments/{policy['approval_environment']}/deployment-branch-policies")
    if not any(rule["name"] == "main" and rule.get("type", "branch") == "branch"
               for rule in branch_rules.get("branch_policies", [])):
        raise ReleaseError("The stable approval environment must allow only the main branch")


def make_plan(github, policy, channel, version, source, reason="", now=None):
    validate_policy(policy)
    if github.repository != policy["module"].removeprefix("github.com/"):
        raise ReleaseError("The module specified by the policy does not match the current repository")
    now = now or dt.datetime.now(dt.timezone.utc)
    base, actual_channel, _ = parse_tag(version)
    if base != policy["next_version"] or channel != actual_channel:
        raise ReleaseError("Target version and channel do not match the policy next_version")
    reason = reason.strip()
    if reason and len(reason) < 20:
        raise ReleaseError("Provide an emergency observation period override reason of at least 20 characters")
    if github.tag_sha(version) is not None:
        raise ReleaseError("Existing tags must not be moved or reused")
    if channel != "stable" and github.tag_sha(base) is not None:
        raise ReleaseError("Prereleases cannot be added to an already published stable version")
    if channel != "stable":
        siblings = github.api(f"git/matching-refs/tags/{base}-{channel}.")
        numbers = [parse_tag(item["ref"].removeprefix("refs/tags/"))[2] for item in siblings
                   if TAG.fullmatch(item["ref"].removeprefix("refs/tags/"))]
        if numbers and parse_tag(version)[2] <= max(numbers):
            raise ReleaseError("The new channel sequence number must exceed the existing number")
    branch = policy["branches"][channel]
    source_sha = github.api(f"git/ref/heads/{branch}")["object"]["sha"]
    if not SHA.fullmatch(source_sha):
        raise ReleaseError("Channel branch does not point to a valid commit")
    source_tag, upstream_sha, promotion_pr, source_evidence = "", "", None, None
    if channel == "preview":
        if not SHA.fullmatch(source) or source != source_sha:
            raise ReleaseError("Preview source requires the full commit SHA of the preview branch HEAD")
    else:
        source_tag = source
        source_base, _, _ = parse_tag(source)
        if source_base != base:
            raise ReleaseError("A candidate for a different version cannot be promoted")
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
            raise ReleaseError("Base dependency versions cannot change during promotion. Publish a new Preview.")
        require_age(artifact["created_at"], policy[f"{expected}_hours"], now, reason)
    tree = github.api(f"git/commits/{source_sha}")["tree"]["sha"]
    source_policy = github.api(f"contents/.github/release-policy.json?ref={source_sha}")
    if json.loads(base64.b64decode(source_policy["content"])) != policy:
        raise ReleaseError("The candidate commit release policy differs from the current plan. Check the target SHA.")
    if channel == "stable":
        approval_environment(github, policy)
        stable = [r for r in github.releases() if not r.get("draft") and not r.get("prerelease")
                  and TAG.fullmatch(r["tag_name"]) and parse_tag(r["tag_name"])[1] == "stable"]
        if stable:
            highest = max(tuple(map(int, parse_tag(r["tag_name"])[0][1:].split("."))) for r in stable)
            if tuple(map(int, base[1:].split("."))) <= highest:
                raise ReleaseError("Only versions newer than existing stable versions can be published")
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
        raise ReleaseError("The verified commit does not match the publication target")
    if os.environ.get("GITHUB_REF") != "refs/heads/" + plan["branch"]:
        raise ReleaseError("The release workflow branch does not match the channel")
    current = github.api("contents/.github/release-policy.json?ref=" + plan["branch"])
    if json.loads(base64.b64decode(current["content"])) != policy:
        raise ReleaseError("The channel branch release policy changed after the plan was created")
    # 승인 대기 중 태그, 정책, 관찰 기간이 바뀌었는지 쓰기 직전에 다시 검사합니다.
    fresh = make_plan(github, policy, plan["channel"], plan["version"],
                      plan["source_tag"] or plan["source_sha"], plan["observation_override_reason"])
    for key in ("source_sha", "source_tree", "source_fingerprint", "kind", "module", "branch", "promotion_pr", "source_evidence", "channel", "version", "components"):
        if fresh[key] != plan[key]:
            raise ReleaseError("The approved plan changed. Start a new run.")
    github.api("git/refs", method="POST", data={"ref": f"refs/tags/{plan['version']}", "sha": verified_sha})


def finish(github, plan, report, manifest_path="release-manifest.json"):
    if (report.get("source_sha") != plan["source_sha"] or report.get("version") != plan["version"] or
            report.get("result") != "passed" or report.get("components") != plan["components"] or
            report.get("scope") != ("cohort" if plan["kind"] == "core" else "platform") or
            not selected_modules_match(plan, report, report.get("scope")) or
            github.tag_sha(plan["version"]) != plan["source_sha"]):
        raise ReleaseError("Public module verification results do not match the publication plan")
    if plan["kind"] == "core":
        policy = json.loads(base64.b64decode(github.api(f"contents/.github/release-policy.json?ref={plan['source_sha']}")["content"]))
        fresh = complete_plan(github, policy, plan["version"])
        for key in ("source_sha", "source_fingerprint", "components", "bootstrap_evidence", "platform_evidence"):
            if plan.get(key) != fresh.get(key):
                raise ReleaseError("Candidate evidence changed during component verification")
    manifest = dict(plan, validation=report)
    path = Path(manifest_path).resolve()
    path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    # 후보는 태그와 Actions artifact로 검증 기록을 남기고 정식 버전만 GitHub Release를 공개합니다.
    if plan["channel"] != "stable":
        return
    with tempfile.TemporaryDirectory(prefix="spine-release-publish-") as directory:
        notes = Path(directory) / "notes.md"
        notes.write_text(f"Commit: `{plan['source_sha']}`\n\n"
                         f"Channel branch: `{plan['branch']}`\n\n"
                         f"Previous stage: `{plan['source_tag'] or 'preview'}`\n\n"
                         f"Migration guide: https://github.com/{github.repository}/blob/{plan['source_sha']}/docs/migration/{parse_tag(plan['version'])[0]}.md\n\n"
                         f"Observation period override reason: {plan['observation_override_reason'] or 'none'}\n\n"
                         "See the attached manifest for verified core and platform versions and execution evidence. Examples are updated after the stable release.\n")
        args = ["gh", "release", "create", plan["version"], str(path), "--repo", github.repository,
                "--verify-tag", "--title", plan["version"], "--notes-file", str(notes), "--latest"]
        run(*args)


def main():
    parser = argparse.ArgumentParser(description='Verify release provenance and observation periods, and publish approved plans.')
    parser.add_argument("command", choices=["plan", "tag", "record-core", "complete-plan", "finish", "validate-policy", "check-environment", "check-module"])
    parser.add_argument("--channel", choices=["preview", "rc", "stable"], default="preview")
    parser.add_argument("--root", default=".")
    parser.add_argument("--version")
    parser.add_argument("--policy", default=".github/release-policy.json")
    parser.add_argument("--plan", default="release-plan.json")
    parser.add_argument("--report", default="release-validation.json")
    parser.add_argument("--manifest", default="release-manifest.json")
    args = parser.parse_args()
    policy = json.loads(Path(args.policy).read_text(encoding="utf-8"))
    validate_policy(policy)
    if args.command == "validate-policy":
        root = Path(__file__).resolve().parents[2]
        if policy["kind"] == "core":
            bootstrap = (root / "internal/bootstrap/bootstrap.go").read_text(encoding="utf-8")
            if f'spineVersion = "{policy["next_version"]}"' not in bootstrap:
                raise ReleaseError("Runtime development base version does not match next_version")
        if not (root / "docs/migration" / (policy["next_version"]+".md")).is_file():
            raise ReleaseError("A migration guide for the target version is required")
        names = ("README.md", "README.en.md", "README.ja.md", "README.zh-Hans.md", "README.zh-Hant.md") if policy["kind"] == "core" else ("README.md",)
        for name in names:
            if f'`{policy["next_version"]}`' not in (root / name).read_text(encoding="utf-8"):
                raise ReleaseError(f"{name}: check the target version")
        print("Release policy format verified")
        return
    if args.command == "check-module":
        check_module(policy, args.channel, Path(args.root))
        return
    github = GitHub(os.environ["GITHUB_REPOSITORY"])
    if args.command == "check-environment":
        approval_environment(github, policy)
        print("Stable release approval environment verified")
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
            raise ReleaseError("Run the workflow from the preview, rc or main branch corresponding to the channel")
        source = os.environ["RELEASE_SOURCE"]
        version = parse_tag(source)[0] if channel == "stable" else os.environ["RELEASE_VERSION"]
        check_module(policy, channel, Path(args.root))
        if policy["kind"] == "core" and version != components(policy, channel)["core_version"]:
            raise ReleaseError("The core candidate tag does not match the core tag in the channel component versions")
        plan = make_plan(github, policy, channel, version, source, os.environ.get("RELEASE_OVERRIDE_REASON", ""))
        Path(args.plan).write_text(json.dumps(plan, ensure_ascii=False, indent=2) + "\n")
        if output := os.environ.get("GITHUB_OUTPUT"):
            with open(output, "a") as stream:
                stream.write(f"sha={plan['source_sha']}\nversion={plan['version']}\nkind={plan['kind']}\n")
        if summary := os.environ.get("GITHUB_STEP_SUMMARY"):
            with open(summary, "a") as stream:
                stream.write(f"Release plan: `{plan['version']}` / `{plan['source_sha']}`\n\n"
                             f"Observation period override reason: {plan['observation_override_reason'] or 'none'}\n")
    else:
        plan = json.loads(Path(args.plan).read_text(encoding="utf-8"))
        if args.command == "tag":
            publish(github, policy, plan, os.environ["VERIFIED_SHA"])
        elif args.command == "record-core":
            report = json.loads(Path(args.report).read_text(encoding="utf-8"))
            if (plan["kind"] != "core" or report.get("scope") != "core" or report.get("result") != "passed" or
                    report.get("components") != plan["components"] or report.get("source_sha") != plan["source_sha"] or
                    report.get("version") != plan["version"] or github.tag_sha(plan["version"]) != plan["source_sha"]):
                raise ReleaseError("Public verification results for the core candidate do not match the plan")
            if not selected_modules_match(plan, report, "core"):
                raise ReleaseError("The public consumer did not select the planned core version")
            Path(args.manifest).write_text(json.dumps(dict(plan, validation=report), ensure_ascii=False, indent=2)+"\n")
        else:
            finish(github, plan, json.loads(Path(args.report).read_text(encoding="utf-8")), args.manifest)


if __name__ == "__main__":
    try:
        main()
    except (ReleaseError, KeyError, ValueError) as exc:
        print(f"Release aborted: {exc}", file=sys.stderr)
        sys.exit(1)
