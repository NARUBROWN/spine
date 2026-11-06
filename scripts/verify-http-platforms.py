#!/usr/bin/env python3
"""net/http 플랫폼과 Echo 비교 E2E를 로컬 소스로 검증합니다."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest() if path.exists() else None


def summary(path):
    tests = {}
    for line in path.read_text().splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if "Test" in event and event.get("Action") in ("pass", "fail", "skip"):
            tests[event["Package"], event["Test"]] = event["Action"]
    leaves = [action for (package, name), action in tests.items()
              if not any(p == package and n.startswith(name + "/") for p, n in tests)]
    return {action: leaves.count(action) for action in ("pass", "fail", "skip")}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--nethttp-root", required=True, type=Path)
    parser.add_argument("--echo-root", required=True, type=Path)
    parser.add_argument("--output-dir", required=True, type=Path)
    args = parser.parse_args()
    core = Path(__file__).resolve().parent.parent
    nethttp, echo = args.nethttp_root.resolve(), args.echo_root.resolve()
    for root in (nethttp, echo):
        if not (root / "go.mod").is_file():
            parser.error(str(root) + " requires a Go module")
    output = args.output_dir.resolve()
    output.mkdir(parents=True, exist_ok=True)
    modules = {"nethttp": nethttp, "platform-e2e": core / "test/http-platform"}
    manifests = [root / name for root in (core, echo, *modules.values()) for name in ("go.mod", "go.sum")]
    before = {str(path): digest(path) for path in manifests}
    results = {}
    with tempfile.TemporaryDirectory(prefix="spine-http-platforms-") as tmp:
        for name, root in modules.items():
            modfile = Path(tmp) / (name + ".mod")
            modfile.write_bytes((root / "go.mod").read_bytes())
            if (root / "go.sum").exists():
                modfile.with_suffix(".sum").write_bytes((root / "go.sum").read_bytes())
            env = os.environ | {"GOWORK": "off", "GOTOOLCHAIN": "go1.25.5"}
            subprocess.run(["go", "mod", "edit", "-modfile=" + str(modfile),
                            "-replace=github.com/NARUBROWN/spine=" + str(core),
                            "-replace=github.com/NARUBROWN/spine-echo-platform=" + str(echo),
                            "-replace=github.com/NARUBROWN/spine-nethttp-platform=" + str(nethttp)],
                           cwd=root, env=env, check=True)
            env["GOFLAGS"] = "-mod=mod -modfile=" + str(modfile)
            log = output / (name + ".jsonl")
            with log.open("w") as stream:
                result = subprocess.run(["go", "test", "-json", "-race", "-count=1", "-timeout=120s", "./..."],
                                        cwd=root, env=env, stdout=stream, stderr=subprocess.STDOUT)
            results[name] = {"exit_code": result.returncode, **summary(log)}
            if result.returncode == 0:
                with (output / (name + "-vet.log")).open("w") as stream:
                    vet = subprocess.run(["go", "vet", "./..."], cwd=root, env=env, stdout=stream, stderr=subprocess.STDOUT)
                results[name]["vet_exit_code"] = vet.returncode
            print(json.dumps({name: results[name]}, ensure_ascii=False), flush=True)
    results["manifests_unchanged"] = before == {str(path): digest(path) for path in manifests}
    results["success"] = results["manifests_unchanged"] and all(
        result["exit_code"] == 0 and result.get("vet_exit_code") == 0
        for result in results.values() if isinstance(result, dict))
    (output / "summary.json").write_text(json.dumps(results, ensure_ascii=False, indent=2) + "\n")
    return 0 if results["success"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
