#!/usr/bin/env python3
"""공개 모듈만으로 후보 코어, CLI 생성 앱과 고정한 예제를 검증합니다."""

import argparse
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time
import urllib.request

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
        raise ReleaseError("공개 검증은 실제 테스트 실행이 필요하며 실패·skip을 허용하지 않습니다")


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
    if CORE in expected and CORE not in found:
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
    return modules(directory, env, expected)


def checkout_example(work, name, spec):
    destination = work / name
    run("git", "clone", "--no-checkout", "--filter=blob:none", "https://github.com/"+spec["repository"]+".git", str(destination))
    run("git", "-C", str(destination), "checkout", "--detach", spec["ref"])
    if run("git", "-C", str(destination), "rev-parse", "HEAD") != spec["ref"]:
        raise ReleaseError("예제 커밋이 manifest와 다릅니다")
    return destination


def require_migrated_example(directory, env):
    parsed = json.loads(subprocess.check_output(["go", "mod", "edit", "-json"], cwd=directory, env=env, text=True))
    versions = {item["Path"]: item["Version"] for item in parsed.get("Require") or []}
    version = versions.get(CORE, "")
    # 제거된 boot.KafkaOptions나 암묵적 WS 선택을 사용하는 옛 예제를 자동 이식하지 않습니다.
    if not version or tuple(map(int, version.split("-")[0].lstrip("v").split("."))) < (0, 6, 5):
        raise ReleaseError(f"{directory}: 공개 예제 참조가 Spine v0.6.5 전환 전입니다. "
                           "수정한 예제를 먼저 커밋·공개하고 release-policy.json의 ref를 갱신하세요")


def health(binary, directory, env, expected_version):
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    process_env = dict(env, SPINE_PORT=str(port))
    with tempfile.TemporaryFile() as log:
        process = subprocess.Popen([str(binary)], cwd=directory, env=process_env, stdout=log, stderr=log)
        try:
            for _ in range(100):
                if process.poll() is not None:
                    break
                try:
                    with urllib.request.urlopen(f"http://127.0.0.1:{port}/health", timeout=1) as response:
                        if response.status == 200:
                            log.seek(0)
                            banner = log.read().decode(errors="replace")
                            if "Spine version: " + expected_version not in banner:
                                raise ReleaseError("런타임 표시 버전이 설치한 후보 버전과 다릅니다: " + banner)
                            return
                except (OSError, TimeoutError):
                    pass
                time.sleep(0.1)
            log.seek(0)
            raise ReleaseError("생성 앱의 HTTP health 확인 실패:\n"+log.read().decode(errors="replace"))
        finally:
            process.terminate()
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
                raise ReleaseError("생성 앱이 종료 제한 시간 안에 종료하지 않았습니다")


def verify(plan, source, work, env, query):
    for key in ("SPINE_TEST_KAFKA_BROKER", "SPINE_TEST_RABBITMQ_URL"):
        if not env.get(key):
            raise ReleaseError(f"{key} 없이 브로커 E2E를 건너뛸 수 없습니다")
    core = json.loads(subprocess.check_output(["go", "list", "-m", "-json", CORE+"@"+query], cwd=work, env=env, text=True))["Version"]
    expected = {CORE: core, **plan["components"]["modules"]}

    # 예제 출처가 준비되지 않았으면 태그를 만들기 전에 종료합니다.
    chat = checkout_example(work, "chat", plan["components"]["examples"]["chat"])
    msa = checkout_example(work, "msa", plan["components"]["examples"]["msa"])
    for directory in (chat, msa/"order-app", msa/"stock-app"):
        require_migrated_example(directory, env)

    public_tests = work / "platform-tests"
    shutil.copytree(source/"test/platform-e2e", public_tests)
    selected = pin(public_tests, env, expected)
    go_tests(["-race", "./...", "-count=1", "-timeout=4m"], public_tests, env)

    execute(["go", "install", "github.com/NARUBROWN/spine-cli/cmd/spine@"+plan["components"]["cli_version"]], work, env)
    cli = str(work / "bin/spine")
    for command in ("schema", "capabilities"):
        envelope = json.loads(run(cli, command, "--json", cwd=work, env=env))
        if not envelope.get("ok"):
            raise ReleaseError("CLI 기능 발견 실패")
    cases = {
        "http": [],
        "websocket": ["--websocket-platform", "gorilla"],
        "kafka-worker": ["--platform", "none", "--event-platform", "kafka", "--validator", "playground"],
        "rabbit-worker": ["--platform", "none", "--event-platform", "rabbitmq", "--validator", "playground"],
        "combined": ["--websocket-platform", "gorilla", "--event-platform", "kafka", "--event-platform", "rabbitmq", "--validator", "playground"],
    }
    for name, options in cases.items():
        args = [cli, "new", name, "--module", "example.com/"+name, "--skip-install", *options]
        generated = json.loads(run(*args, "--dry-run", "--json", cwd=work, env=env))
        if not generated.get("ok") or not generated["result"].get("base_hash"):
            raise ReleaseError("CLI 생성 계획 확인 실패")
        applied = json.loads(run(*args, "--if-match", generated["result"]["base_hash"], "--json", cwd=work, env=env))
        if not applied.get("ok"):
            raise ReleaseError("CLI 생성 계획 적용 실패")
        app = work / name
        used = pin(app, env, expected)
        # 기본 레시피가 후보 코어를 선택했다고 오인하지 않도록 실제 사용한 버전을 기록합니다.
        selected.update(used)
        execute([cli, "contract", "check", "--root", str(app), "--warnings-as-errors", "--json"], work, env)
        execute([cli, "verify", "--root", str(app), "--warnings-as-errors", "--json"], work, env)
        execute([cli, "contract", "probe", "--root", str(app), "--json"], work, env)
        if "--platform" not in options:
            binary = app / "server"
            execute(["go", "build", "-race", "-o", str(binary), "."], app, env)
            health(binary, app, env, core)

    pin(chat, env, expected)
    go_tests(["-race", "./...", "-count=1"], chat, env)
    go_tests(["-race", "-tags=e2e", "./e2e", "-count=1", "-timeout=3m"], chat, env)
    for directory in (msa/"order-app", msa/"stock-app"):
        pin(directory, env, expected)
        execute([cli, "verify", "--root", str(directory), "--warnings-as-errors", "--json"], work, env)
        go_tests(["-race", "./...", "-count=1"], directory, env)
    msa_env = dict(env, GOWORK=str(msa/"go.work"), SPINE_DEMO_E2E_BROKER=env["SPINE_TEST_KAFKA_BROKER"])
    modules(msa/"order-app", msa_env, expected)
    modules(msa/"stock-app", msa_env, expected)
    go_tests(["-race", "-tags=e2e", "./e2e", "-count=1", "-timeout=4m"], msa, msa_env)
    return {"result": "passed", "source_sha": plan["source_sha"], "version": query,
            "resolved_core_version": core, "go_version": run("go", "version", env=env), "components": plan["components"], "selected_modules": selected,
            "evidence": ["public modules without core/platform replace", "HTTP/WS/validator and two live brokers",
                         "five CLI generation combinations", "pinned chat and MSA repositories"]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plan", required=True)
    parser.add_argument("--source", required=True)
    parser.add_argument("--query", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    plan = json.loads(Path(args.plan).read_text())
    with tempfile.TemporaryDirectory(prefix="spine-public-release-") as directory:
        work = Path(directory)
        env = dict(os.environ, GOWORK="off", GOMODCACHE=str(work/"modules"), GOBIN=str(work/"bin"), GOFLAGS="-modcacherw",
                   GOPROXY="https://proxy.golang.org,direct", GOSUMDB="sum.golang.org", GOPRIVATE="", GONOSUMDB="")
        env.update(SPINE_KAFKA_BROKERS=env.get("SPINE_TEST_KAFKA_BROKER", ""), SPINE_KAFKA_GROUP_ID="release-verification",
                   SPINE_KAFKA_ALLOW_INSECURE="true", SPINE_RABBITMQ_URL=env.get("SPINE_TEST_RABBITMQ_URL", ""),
                   SPINE_RABBITMQ_EXCHANGE="release-verification", SPINE_RABBITMQ_ALLOW_INSECURE="true")
        report = verify(plan, Path(args.source).resolve(), work, env, args.query)
        Path(args.output).write_text(json.dumps(report, ensure_ascii=False, indent=2)+"\n")


if __name__ == "__main__":
    main()
