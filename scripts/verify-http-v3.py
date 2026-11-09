#!/usr/bin/env python3
"""공개하지 않은 코어/Echo 소스를 임시 main module 설정으로 검증합니다."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest() if path.exists() else None


def test_summary(path):
    events = []
    for line in path.read_text().splitlines():
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    tests = {(e.get("Package"), e["Test"]): e["Action"] for e in events
             if "Test" in e and e.get("Action") in ("pass", "fail", "skip")}
    leaves = [action for (package, name), action in tests.items()
              if not any(p == package and n.startswith(name + "/") for p, n in tests)]
    return {action: leaves.count(action) for action in ("pass", "fail", "skip")}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--echo-root", required=True, type=Path)
    parser.add_argument("--output-dir", required=True, type=Path)
    args = parser.parse_args()
    core = Path(__file__).resolve().parent.parent
    echo = args.echo_root.resolve()
    output = args.output_dir.resolve()
    if not (echo / "go.mod").is_file():
        parser.error("--echo-root requires the Echo platform module")
    output.mkdir(parents=True, exist_ok=True)
    modules = {"core": core, "echo": echo, "integration": core / "test", "openapi": core / "test/openapi"}
    manifests = [root / name for root in modules.values() for name in ("go.mod", "go.sum")]
    before = {str(path): digest(path) for path in manifests}
    results = {}
    with tempfile.TemporaryDirectory(prefix="spine-http-v3-") as tmp:
        for name, root in modules.items():
            env = os.environ.copy()
            env["GOWORK"] = "off"
            if name != "core":
                modfile = Path(tmp) / (name + ".mod")
                modfile.write_bytes((root / "go.mod").read_bytes())
                if (root / "go.sum").exists():
                    modfile.with_suffix(".sum").write_bytes((root / "go.sum").read_bytes())
                # 라이브러리 go.mod와 캐시는 바꾸지 않습니다. 하위 go build도 같은 조합을 씁니다.
                subprocess.run(["go", "mod", "edit", "-modfile=" + str(modfile),
                                "-replace=github.com/NARUBROWN/spine=" + str(core),
                                "-replace=github.com/NARUBROWN/spine-echo-platform=" + str(echo)],
                               cwd=root, env=env, check=True)
                env["GOFLAGS"] = env.get("GOFLAGS", "") + " -modfile=" + str(modfile) + " -mod=mod"
            test = ["go", "run", "./scripts/test", "test"] if name == "core" else ["go", "test"]
            log = output / (name + ".jsonl")
            with log.open("w") as stream:
                result = subprocess.run(test + ["-json", "-race", "-count=1", "-timeout=120s", "./..."],
                                        cwd=root, env=env, stdout=stream, stderr=subprocess.STDOUT)
            results[name] = {"exit_code": result.returncode, **test_summary(log)}
            if result.returncode == 0:
                vet = ["go", "run", "./scripts/test", "vet"] if name == "core" else ["go", "vet"]
                with (output / (name + "-vet.log")).open("w") as stream:
                    checked = subprocess.run(vet + ["./..."], cwd=root, env=env,
                                             stdout=stream, stderr=subprocess.STDOUT)
                results[name]["vet_exit_code"] = checked.returncode
            print(json.dumps({name: results[name]}, ensure_ascii=False), flush=True)
    after = {str(path): digest(path) for path in manifests}
    results["manifests_unchanged"] = before == after
    success = before == after and all(value["exit_code"] == 0 and value["pass"] > 0 and value.get("vet_exit_code") == 0
                                     for value in results.values() if isinstance(value, dict))
    results["success"] = success
    (output / "summary.json").write_text(json.dumps(results, ensure_ascii=False, indent=2) + "\n")
    return 0 if success else 1


if __name__ == "__main__":
    raise SystemExit(main())
