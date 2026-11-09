#!/usr/bin/env python3
"""정식 발행을 마친 뒤 별도로 갱신한 공개 예제 커밋을 검증합니다."""

import argparse
import base64
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

from release import CORE, GitHub, ReleaseError, SHA, parse_tag, verify_source_candidate
from verify_public import checkout_example, execute, go_tests, modules


def verify_examples(policy, version, work, env, github):
    if parse_tag(version)[1] != "stable" or os.environ.get("GITHUB_REF") != "refs/heads/main":
        raise ReleaseError("Public example verification requires a stable tag and the main branch")
    for key in ("SPINE_TEST_KAFKA_BROKER", "SPINE_TEST_RABBITMQ_URL"):
        if not env.get(key):
            raise ReleaseError("Example verification requires both brokers")
    sha = github.tag_sha(version)
    tagged = json.loads(base64.b64decode(github.api(f"contents/.github/release-policy.json?ref={sha}")["content"]))
    _, manifest = verify_source_candidate(github, version, "stable", tagged)
    expected = {CORE: version, **manifest["components"]["modules"]}
    specs = policy.get("post_release_examples", {})
    if set(specs) != {"chat", "msa"}:
        raise ReleaseError("Specify chat and MSA example commits to verify after the stable release")
    for spec in specs.values():
        if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", spec["repository"]) or not SHA.fullmatch(spec["ref"]):
            raise ReleaseError("Pin the example repository and full commit SHA")
    chat = checkout_example(work, "chat", specs["chat"])
    msa = checkout_example(work, "msa", specs["msa"])
    for directory in (chat, msa / "order-app", msa / "stock-app"):
        parsed = json.loads(subprocess.check_output(["go", "mod", "edit", "-json"], cwd=directory, env=env, text=True))
        declared = {item["Path"]: item["Version"] for item in (parsed.get("Require") or [])}
        if declared.get(CORE) != version or any(declared[name] != value for name, value in expected.items() if name in declared):
            raise ReleaseError("Publish a commit updating the example go.mod to the stable component versions first: " + str(directory))
        execute(["go", "mod", "download"], directory, env)
        execute(["go", "mod", "verify"], directory, env)
        modules(directory, env, {name: value for name, value in expected.items() if name in declared})
        go_tests(["-race", "./...", "-count=1"], directory, env)
    go_tests(["-race", "-tags=e2e", "./e2e", "-count=1", "-timeout=3m"], chat, env)
    msa_env = dict(env, GOWORK=str(msa / "go.work"), SPINE_DEMO_E2E_BROKER=env["SPINE_TEST_KAFKA_BROKER"])
    go_tests(["-race", "-tags=e2e", "./e2e", "-count=1", "-timeout=4m"], msa, msa_env)
    return {"result": "passed", "version": version, "examples": specs, "components": manifest["components"]}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description='Verify updated public example commits after a stable release.')
    parser.add_argument("--version", required=True)
    args = parser.parse_args()
    policy = json.loads(Path(".github/release-policy.json").read_text())
    with tempfile.TemporaryDirectory(prefix="spine-post-release-examples-") as temporary:
        work = Path(temporary)
        env = dict(os.environ, GOWORK="off", GOMODCACHE=str(work / "modules"), GOFLAGS="-modcacherw")
        report = verify_examples(policy, args.version, work, env, GitHub(os.environ["GITHUB_REPOSITORY"]))
        Path("post-release-examples.json").write_text(json.dumps(report, ensure_ascii=False, indent=2)+"\n")
