#!/usr/bin/env python3
"""Synchronize platform release inputs from a verified core candidate."""

import argparse
import base64
import difflib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

from release import CORE, GitHub, ReleaseError, SHA, components, parse_tag, validate_policy, verify_source_candidate


def candidate(tag, github=None):
    github = github or GitHub("NARUBROWN/spine")
    _, channel, _ = parse_tag(tag)
    sha = github.tag_sha(tag)
    if not SHA.fullmatch(sha or ""):
        raise ReleaseError("Core candidate tag is missing")
    raw = github.api(f"contents/.github/release-policy.json?ref={sha}")
    policy = json.loads(base64.b64decode(raw["content"]))
    validate_policy(policy)
    if policy["kind"] != "core" or components(policy, channel)["core_version"] != tag:
        raise ReleaseError("Core candidate tag does not match its release policy")
    _, manifest = verify_source_candidate(github, tag, channel, policy, bootstrap=True)
    return sha, channel, manifest["components"]


def prepare(root, sha, channel, selected, destination, tidy=True):
    root = root.resolve()
    if subprocess.check_output(["git", "status", "--porcelain", "--untracked-files=all"], cwd=root, text=True).strip():
        raise ReleaseError("Platform working tree must be clean: " + str(root))
    head = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=root, text=True).strip()
    original = json.loads((root / ".github/release-policy.json").read_text())
    validate_policy(original)
    module = original["module"]
    if original["kind"] != "platform" or module not in selected["modules"]:
        raise ReleaseError("Platform is absent from the verified core combination: " + module)
    target = selected["modules"][module]
    if parse_tag(target)[0] != original["next_version"]:
        raise ReleaseError("Platform next_version differs from the core combination: " + module)
    archive = subprocess.run(["git", "archive", "HEAD"], cwd=root, capture_output=True, check=True).stdout
    import io
    import tarfile
    with tarfile.open(fileobj=io.BytesIO(archive)) as package:
        package.extractall(destination, filter="data")
    policy_path = destination / ".github/release-policy.json"
    updated = json.loads(policy_path.read_text())
    updated["channels"][channel]["core_version"] = selected["core_version"]
    updated["ci_core"] = sha
    validate_policy(updated)
    policy_path.write_text(json.dumps(updated, ensure_ascii=False, indent=2) + "\n")
    command = ["python3", "scripts/release/pin_channel.py", "--channel", channel, "--apply"]
    if tidy:
        command.append("--tidy")
    subprocess.run(command, cwd=destination, env=dict(os.environ, GOWORK="off"), check=True, stdout=subprocess.DEVNULL)
    subprocess.run(["python3", "scripts/release/release.py", "check-module", "--channel", channel],
                   cwd=destination, env=dict(os.environ, GOWORK="off"), check=True, stdout=subprocess.DEVNULL)
    directories = [Path(".")]
    directories += [Path(name) for name in ("e2e", "examples/worker") if (destination / name / "go.mod").is_file()]
    paths = [Path(".github/release-policy.json")]
    for directory in directories:
        paths.extend(directory / name for name in ("go.mod", "go.sum") if (destination / directory / name).exists())
    changes = {}
    for path in paths:
        before = (root / path).read_bytes() if (root / path).exists() else b""
        after = (destination / path).read_bytes()
        if before != after:
            changes[path] = (before, after)
    return module, target, head, changes


def synchronize(tag, parent, apply=False, tidy=True, github=None):
    sha, channel, selected = candidate(tag, github)
    with tempfile.TemporaryDirectory(prefix="spine-platform-sync-") as temporary:
        jobs = []
        for index, module in enumerate(sorted(selected["modules"])):
            root = (parent / module.removeprefix("github.com/NARUBROWN/")).resolve()
            if not (root / ".git").exists():
                raise ReleaseError("Platform checkout is missing: " + str(root))
            destination = Path(temporary) / str(index)
            destination.mkdir()
            name, version, head, changes = prepare(root, sha, channel, selected, destination, tidy)
            jobs.append((root, name, version, head, changes))
        for root, name, version, _, changes in jobs:
            print(f"{name}: {version}, core {tag}, source {sha}")
            for path, (before, after) in changes.items():
                print("".join(difflib.unified_diff(before.decode().splitlines(True), after.decode().splitlines(True),
                                                   fromfile=str(root / path), tofile=str(root / path))), end="")
        if apply:
            if candidate(tag, github) != (sha, channel, selected):
                raise ReleaseError("Core candidate changed while preparing platform updates")
            for root, _, _, head, _ in jobs:
                current = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=root, text=True).strip()
                if current != head or subprocess.check_output(["git", "status", "--porcelain", "--untracked-files=all"], cwd=root, text=True).strip():
                    raise ReleaseError("Platform working tree changed while preparing: " + str(root))
            written = []
            try:
                for root, _, _, _, changes in jobs:
                    for path, (before, after) in changes.items():
                        target = root / path
                        existed = target.exists()
                        target.write_bytes(after)
                        written.append((target, before, existed))
            except Exception:
                for target, before, existed in reversed(written):
                    if existed:
                        target.write_bytes(before)
                    else:
                        target.unlink(missing_ok=True)
                raise
    return sha


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--core-tag", required=True)
    parser.add_argument("--platform-parent", type=Path, required=True)
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--no-tidy", action="store_true")
    args = parser.parse_args()
    synchronize(args.core_tag, args.platform_parent, args.apply, not args.no_tidy)


if __name__ == "__main__":
    try:
        main()
    except (ReleaseError, subprocess.CalledProcessError, OSError, ValueError) as exc:
        raise SystemExit("Release sync aborted: " + str(exc))
