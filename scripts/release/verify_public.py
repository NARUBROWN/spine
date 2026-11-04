#!/usr/bin/env python3
"""공개 코어 후보, 플랫폼 또는 같은 채널의 전체 조합을 검증합니다."""

import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

from release import ReleaseError, run


CORE = "github.com/NARUBROWN/spine"


def execute(args, directory, env, timeout=600):
    print(f"[{Path(directory).name}] {' '.join(args)}", flush=True)
    subprocess.run(args, cwd=directory, env=env, check=True, timeout=timeout)


def go_tests(options, directory, env):
    command = ["go", "test", "-json", *options]
    print(f"[{Path(directory).name}] {' '.join(command)}", flush=True)
    result = subprocess.run(command, cwd=directory, env=env, capture_output=True, text=True, timeout=600)
    print(result.stdout, end="", flush=True)
    print(result.stderr, end="", flush=True)
    events = []
    for line in result.stdout.splitlines():
        try:
            events.append(json.loads(line))
        except ValueError:
            pass
    if result.returncode or any(e.get("Action") == "skip" for e in events) or not any(e.get("Action") == "run" and e.get("Test") for e in events):
        raise ReleaseError("공개 검증은 실제 테스트 실행이 필요하며 실패나 skip을 허용하지 않습니다")


def modules(directory, env, expected):
    result = subprocess.check_output(["go", "list", "-m", "-json", "all"], cwd=directory, env=env, text=True)
    decoder = json.JSONDecoder()
    found = {}
    while result.strip():
        item, index = decoder.raw_decode(result.lstrip())
        result = result.lstrip()[index:]
        # MSA의 공유 이벤트 모듈만 저장소 내부 소스로 연결할 수 있습니다.
        replacement = item.get("Replace")
        if replacement and item["Path"] != "github.com/NARUBROWN/spine-simple-msa":
            raise ReleaseError(f"공개 검증에 replace가 남아 있습니다: {item['Path']}")
        if item["Path"] in expected:
            if item.get("Version") != expected[item["Path"]] or replacement:
                raise ReleaseError(f"검증 대상 버전 불일치: {item['Path']} {item.get('Version')}")
            found[item["Path"]] = item["Version"]
    if set(expected) - set(found):
        raise ReleaseError("검증 앱에서 후보 코어를 사용하지 않습니다")
    return found


def pin(directory, env, expected):
    parsed = json.loads(subprocess.check_output(["go", "mod", "edit", "-json"], cwd=directory, env=env, text=True))
    for item in parsed.get("Replace") or []:
        if item["Old"]["Path"] != "github.com/NARUBROWN/spine-simple-msa":
            execute(["go", "mod", "edit", "-dropreplace="+item["Old"]["Path"]], directory, env)
    required = {item["Path"] for item in parsed.get("Require") or []}
    for path, version in expected.items():
        if path != CORE and path not in required:
            continue
        execute(["go", "mod", "edit", f"-require={path}@{version}"], directory, env)
    execute(["go", "mod", "tidy"], directory, env)
    execute(["go", "mod", "verify"], directory, env)
    return modules(directory, env, {path: version for path, version in expected.items() if path == CORE or path in required})


def checkout_example(work, name, spec):
    destination = work / name
    run("git", "clone", "--no-checkout", "--filter=blob:none", "https://github.com/"+spec["repository"]+".git", str(destination))
    run("git", "-C", str(destination), "checkout", "--detach", spec["ref"])
    if run("git", "-C", str(destination), "rev-parse", "HEAD") != spec["ref"]:
        raise ReleaseError("예제 커밋이 manifest와 다릅니다")
    return destination


def consumer(work, env, expected, imports):
    directory = work / "consumer"
    directory.mkdir()
    (directory / "go.mod").write_text("module example.com/spine-release-check\n\ngo 1.26.0\n")
    (directory / "main.go").write_text("package main\nimport (\n" + "".join(' _ "' + name + '"\n' for name in imports) + ")\nfunc main() {}\n")
    for module, version in expected.items():
        execute(["go", "mod", "edit", "-require=" + module + "@" + version], directory, env)
    execute(["go", "mod", "tidy"], directory, env)
    execute(["go", "mod", "verify"], directory, env)
    execute(["go", "build", "-mod=readonly", "-o", str(work / "consumer-app"), "."], directory, env)
    return modules(directory, env, expected)


def verify(plan, source, work, env, query, scope="cohort"):
    if scope not in ("core", "platform", "cohort"):
        raise ReleaseError("공개 검증 범위를 지정해야 합니다")
    if scope != "core":
        for key in ("SPINE_TEST_KAFKA_BROKER", "SPINE_TEST_RABBITMQ_URL"):
            if not env.get(key):
                raise ReleaseError(f"{key} 없이 브로커 E2E를 건너뛸 수 없습니다")
    module = plan["module"]
    resolved = json.loads(subprocess.check_output(["go", "list", "-m", "-json", module + "@" + query], cwd=work, env=env, text=True))["Version"]
    selected_components = plan["components"]
    if scope == "core":
        if plan["kind"] != "core":
            raise ReleaseError("코어 후보 검증은 코어에만 적용합니다")
        expected = {CORE: resolved}
    elif scope == "platform":
        if plan["kind"] != "platform":
            raise ReleaseError("플랫폼 검증은 플랫폼에만 적용합니다")
        expected = {CORE: selected_components["core_version"], module: resolved}
    else:
        if plan["kind"] != "core" or resolved != selected_components["core_version"]:
            raise ReleaseError("전체 조합의 코어 버전이 계획과 다릅니다")
        expected = {CORE: resolved, **selected_components["modules"]}
    selected = consumer(work, env, expected, list(expected))
    if scope == "platform":
        downloaded = json.loads(subprocess.check_output(["go", "mod", "download", "-json", module + "@" + query], cwd=work / "consumer", env=env, text=True))
        tests = work / "published-platform"
        shutil.copytree(downloaded["Dir"], tests)
        for path in [tests, *tests.rglob("*")]:
            path.chmod(0o755 if path.is_dir() else 0o644)
        go_tests(["-mod=readonly", "-race", "./...", "-count=1", "-timeout=4m"], tests, env)
        execute(["go", "vet", "-mod=readonly", "./..."], tests, env)
        modules(tests, env, {CORE: selected_components["core_version"]})
    elif scope == "cohort":
        for name in ("platform-e2e", "openapi"):
            tests = work / name
            shutil.copytree(source / "test" / name, tests)
            selected.update(pin(tests, env, expected))
            go_tests(["-mod=readonly", "-race", "./...", "-count=1", "-timeout=4m"], tests, env)
    return {"result": "passed", "scope": scope, "source_sha": plan["source_sha"], "version": query,
            "components": selected_components, "selected_modules": selected,
            "go_version": run("go", "version", env=env),
            "evidence": ["published modules without go.work or replace", "consumer build", "scope=" + scope]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plan", required=True)
    parser.add_argument("--scope", choices=["core", "platform", "cohort"], required=True)
    parser.add_argument("--source", required=True)
    parser.add_argument("--query", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    plan = json.loads(Path(args.plan).read_text())
    with tempfile.TemporaryDirectory(prefix="spine-public-release-") as directory:
        work = Path(directory)
        env = dict(os.environ, GOWORK="off", GOMODCACHE=str(work/"modules"), GOBIN=str(work/"bin"), GOFLAGS="-modcacherw",
                   GOPROXY="https://proxy.golang.org,direct", GOSUMDB="sum.golang.org", GOPRIVATE="", GONOPROXY="", GONOSUMDB="")
        env.update(SPINE_KAFKA_BROKERS=env.get("SPINE_TEST_KAFKA_BROKER", ""), SPINE_KAFKA_GROUP_ID="release-verification",
                   SPINE_KAFKA_ALLOW_INSECURE="true", SPINE_RABBITMQ_URL=env.get("SPINE_TEST_RABBITMQ_URL", ""),
                   SPINE_RABBITMQ_EXCHANGE="release-verification", SPINE_RABBITMQ_ALLOW_INSECURE="true")
        report = verify(plan, Path(args.source).resolve(), work, env, args.query, args.scope)
        Path(args.output).write_text(json.dumps(report, ensure_ascii=False, indent=2)+"\n")


if __name__ == "__main__":
    main()
