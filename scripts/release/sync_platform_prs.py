#!/usr/bin/env python3
"""Open Preview platform PRs from a verified core candidate."""

import argparse
import base64
import json
from pathlib import Path
import subprocess
import tempfile

from release import GitHub, ReleaseError, parse_tag, validate_policy, verify_source_candidate
from sync_platforms import candidate, synchronize


def run(*args, cwd=None):
    result = subprocess.run(args, cwd=cwd, text=True, capture_output=True)
    if result.returncode:
        raise ReleaseError(f"Command failed: {args[0]}: {result.stderr.strip()}")
    return result.stdout.strip()


def promotion_sources(tag, selected, github=None):
    github = github or GitHub("NARUBROWN/spine")
    _, channel, _ = parse_tag(tag)
    if channel == "preview":
        return {}
    expected = "preview" if channel == "rc" else "rc"
    core_sha = github.tag_sha(tag)
    core_policy = json.loads(base64.b64decode(github.api(
        "contents/.github/release-policy.json?ref=" + core_sha)["content"]))
    validate_policy(core_policy)
    _, core_manifest = verify_source_candidate(github, tag, channel, core_policy, bootstrap=True)
    previous = core_manifest.get("source_tag", "")
    if parse_tag(previous)[1] != expected:
        raise ReleaseError("Core candidate does not identify the previous release channel")
    _, previous_manifest = verify_source_candidate(github, previous, expected, core_policy)
    evidence = previous_manifest.get("platform_evidence", {})
    if set(evidence) != set(selected["modules"]):
        raise ReleaseError("Previous core combination lacks verified platform sources")
    for module, item in evidence.items():
        if (item.get("version") != previous_manifest["components"]["modules"][module] or
                parse_tag(item.get("version", ""))[1] != expected):
            raise ReleaseError("Previous platform evidence does not match the core combination: " + module)
    return evidence


def open_prs(tag, workspace):
    sha, channel, selected = candidate(tag)
    sources = promotion_sources(tag, selected) if channel != "preview" else {}
    base = "main" if channel == "stable" else channel
    source_branch = "preview" if channel in ("preview", "rc") else "rc"
    roots = []
    for module in sorted(selected["modules"]):
        repository = module.removeprefix("github.com/")
        root = workspace / repository.split("/")[-1]
        version = selected["modules"][module]
        branch = ("sync/core-" + tag if channel == "preview" else
                  "promote/" + channel + "/" + version)
        run("gh", "repo", "clone", repository, str(root), "--", "--branch", source_branch, "--single-branch")
        if channel != "preview":
            source = sources[module]
            source_tag = source["version"]
            run("git", "fetch", "origin", "refs/tags/" + source_tag + ":refs/tags/" + source_tag, cwd=root)
            commit = run("git", "rev-parse", "refs/tags/" + source_tag + "^{commit}", cwd=root)
            if commit != source.get("source_sha"):
                raise ReleaseError("Platform source tag differs from the verified core combination: " + repository)
            run("git", "checkout", "--detach", commit, cwd=root)
        existing = subprocess.run(["git", "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/" + branch],
                                  cwd=root, text=True, capture_output=True)
        if existing.returncode == 0:
            raise ReleaseError("Target branch already exists; inspect earlier sync before retrying: " + repository)
        if existing.returncode != 2:
            raise ReleaseError("Cannot check target branch: " + repository + ": " + existing.stderr.strip())
        roots.append((repository, root, version, branch))
    if synchronize(tag, workspace, apply=True) != sha:
        raise ReleaseError("Core candidate changed while preparing platform PRs")
    prepared = []
    for repository, root, version, branch in roots:
        if not run("git", "status", "--porcelain", "--untracked-files=all", cwd=root):
            print(repository + ": already synchronized", flush=True)
            continue
        run("git", "checkout", "-b", branch, cwd=root)
        run("git", "add", ".github/release-policy.json", "go.mod", cwd=root)
        if (root / "go.sum").exists():
            run("git", "add", "go.sum", cwd=root)
        for directory in ("e2e", "examples/worker"):
            for filename in ("go.mod", "go.sum"):
                path = root / directory / filename
                if path.exists():
                    run("git", "add", str(path.relative_to(root)), cwd=root)
        run("git", "-c", "user.name=github-actions[bot]", "-c",
            "user.email=41898282+github-actions[bot]@users.noreply.github.com",
            "commit", "-m", "릴리스: 코어 " + tag + " 조합 동기화", cwd=root)
        prepared.append((repository, root, version, branch))
    for repository, root, version, branch in prepared:
        run("git", "push", "origin", "HEAD:refs/heads/" + branch, cwd=root)
        body = (f"검증된 코어 후보 `{tag}`의 릴리스 조합에 맞춰 코어 의존성, 체크섬과 "
                f"소스 CI SHA를 동기화합니다. 플랫폼의 예정 태그는 `{version}`이며 "
                f"코어 소스는 `{sha}`입니다. 플랫폼 CI와 공개 발행 검증은 기존 절차대로 수행합니다.\n")
        with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", suffix=".md") as body_file:
            body_file.write(body)
            body_file.flush()
            link = run("gh", "pr", "create", "-R", repository, "--base", base, "--head", branch,
                       "--title", "릴리스: 코어 " + tag + " 조합 동기화", "--body-file", body_file.name, cwd=root)
        print(repository + ": " + link, flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--core-tag", required=True)
    parser.add_argument("--workspace", type=Path, required=True)
    args = parser.parse_args()
    args.workspace.mkdir(parents=True, exist_ok=True)
    open_prs(args.core_tag, args.workspace.resolve())


if __name__ == "__main__":
    try:
        main()
    except (ReleaseError, OSError, ValueError) as exc:
        raise SystemExit("Platform PR sync aborted: " + str(exc))
