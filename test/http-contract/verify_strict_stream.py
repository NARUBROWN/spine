#!/usr/bin/env python3
"""분리된 fasthttp 복사본의 EOF 패치 전후를 같은 테스트로 비교합니다."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile


MODULE = Path(__file__).resolve().parent
CORE = MODULE.parents[1]
EXPECTED_STREAMING_SHA256 = "910be3224394de885c51cbea3aeb54eb72e38702f0a7d4c83a9e3207273a3777"
EXPECTED_FAILURES = {
    "TestTruncatedRequestPreservesReadFailure/fasthttp/chunk-terminator",
    "TestTruncatedRequestPreservesReadFailure/fasthttp/trailer-terminator",
}


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def test_summary(output):
    rows = [json.loads(line) for line in output.splitlines() if line.startswith("{")]
    tests = [r for r in rows if r.get("Test") and r.get("Action") in ("pass", "fail", "skip")]
    names = {r["Test"] for r in tests}
    leaves = [r for r in tests if not any(n.startswith(r["Test"] + "/") for n in names)]
    return {
        "leaf_pass": sum(r["Action"] == "pass" for r in leaves),
        "leaf_fail": sum(r["Action"] == "fail" for r in leaves),
        "skip": sum(r["Action"] == "skip" for r in tests),
        "failed_cases": sorted({r["Test"] for r in leaves if r["Action"] == "fail"}),
        "package_pass": sum(r.get("Action") == "pass" and "Test" not in r for r in rows),
    }


def run(args, cwd, env, output, name, results, timeout=180):
    print(json.dumps({"step": name, "status": "running"}), flush=True)
    completed = subprocess.run(args, cwd=cwd, env=env, text=True, capture_output=True, timeout=timeout)
    (output / (name + ".stdout")).write_text(completed.stdout)
    (output / (name + ".stderr")).write_text(completed.stderr)
    result = {"exit_code": completed.returncode}
    if "-json" in args:
        result.update(test_summary(completed.stdout))
    results[name] = result
    print(json.dumps({"step": name, **result}), flush=True)
    return completed


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output-dir", type=Path)
    args = parser.parse_args()
    env = os.environ.copy()
    env["GOWORK"] = "off"
    patch = MODULE / "patches" / "fasthttp-v1.77.0-strict-chunk-eof.patch"
    metadata = subprocess.run(
        ["go", "list", "-m", "-json", "github.com/valyala/fasthttp"],
        cwd=MODULE, env=env, text=True, capture_output=True, check=True,
    )
    dependency = json.loads(metadata.stdout)
    if dependency["Version"] != "v1.77.0" or dependency.get("Replace"):
        raise RuntimeError("순정 fasthttp v1.77.0에서 시작해야 합니다.")
    original = Path(dependency["Dir"]) / "streaming.go"
    original_hash = digest(original)
    if original_hash != EXPECTED_STREAMING_SHA256:
        raise RuntimeError("고정한 원본 streaming.go 해시와 다릅니다.")
    test_hash = digest(MODULE / "contract_test.go")
    core_inputs = {str(p.relative_to(CORE)): digest(p) for p in [CORE / "go.mod", CORE / "go.sum"] if p.exists()}
    contract_inputs = {str(p.relative_to(CORE)): digest(p) for p in (CORE / "contract" / "http" / "v3").glob("*.go")}
    results = {
        "fasthttp_version": dependency["Version"],
        "go_version": subprocess.check_output(["go", "version"], cwd=MODULE, env=env, text=True).strip(),
        "patch_sha256": digest(patch),
        "original_streaming_sha256": original_hash,
        "test_sha256": test_hash,
    }

    with tempfile.TemporaryDirectory(prefix="spine-strict-stream-") as temporary:
        root = Path(temporary)
        output = args.output_dir.resolve() if args.output_dir else root / "results"
        output.mkdir(parents=True, exist_ok=True)
        try:
            baseline = run(
                ["go", "test", "-mod=readonly", "-race", "-json", "-count=1", "-timeout=90s", "./..."],
                MODULE, env, output, "baseline", results,
            )
            if baseline.returncode != 1 or set(results["baseline"]["failed_cases"]) != EXPECTED_FAILURES:
                raise RuntimeError("기준 실행이 알려진 두 실패와 다릅니다. 로그를 확인해야 합니다.")

            engine = root / "fasthttp"
            shutil.copytree(dependency["Dir"], engine, copy_function=shutil.copyfile)
            # 캐시의 읽기 전용 권한을 복사본에서만 바꿔 패치/정리가 가능하게 합니다.
            for folder, _, _ in os.walk(engine):
                os.chmod(folder, 0o700)
            applied = subprocess.run(
                ["git", "apply", "--check", "--unidiff-zero", str(patch)], cwd=engine, text=True, capture_output=True,
            )
            if applied.returncode != 0:
                raise RuntimeError("분리 복사본에 패치를 적용할 수 없습니다: " + applied.stderr)
            subprocess.run(["git", "apply", "--unidiff-zero", str(patch)], cwd=engine, check=True)
            results["patched_streaming_sha256"] = digest(engine / "streaming.go")

            fixture = root / "probe"
            shutil.copytree(MODULE, fixture, ignore=shutil.ignore_patterns("__pycache__", ".git"))
            subprocess.run(
                ["go", "mod", "edit", "-replace=github.com/NARUBROWN/spine=" + str(CORE),
                 "-replace=github.com/valyala/fasthttp=" + str(engine)],
                cwd=fixture, env=env, check=True,
            )
            if digest(fixture / "contract_test.go") != test_hash:
                raise RuntimeError("패치 전후의 테스트가 달라졌습니다.")

            patched = run(
                ["go", "test", "-mod=readonly", "-race", "-json", "-count=1", "-timeout=90s", "./..."],
                fixture, env, output, "patched", results,
            )
            repeated = run(
                ["go", "test", "-mod=readonly", "-race", "-json", "-count=20", "-timeout=60s",
                 "-run=TestTruncatedRequestPreservesReadFailure/^fasthttp$/(chunk-terminator|trailer-terminator)$", "./..."],
                fixture, env, output, "repeated", results,
            )
            vetted = run(["go", "vet", "-mod=readonly", "./..."], fixture, env, output, "vet", results)
            upstream = run(
                ["go", "test", "-mod=readonly", "-race", "-json", "-count=1", "-timeout=90s",
                 "-run=Test.*(Stream|Chunk|Trailer).*", "."],
                engine, env, output, "upstream_stream_tests", results,
            )
            results["success"] = all(r.returncode == 0 for r in [patched, repeated, vetted, upstream])
            results["same_tests_before_after"] = digest(fixture / "contract_test.go") == test_hash
        finally:
            results["module_cache_unchanged"] = digest(original) == original_hash
            results["contract_unchanged"] = all(digest(CORE / name) == value for name, value in contract_inputs.items())
            results["core_module_inputs_unchanged"] = all(digest(CORE / name) == value for name, value in core_inputs.items())
            results["fixture_tests_unchanged"] = digest(MODULE / "contract_test.go") == test_hash
            (output / "summary.json").write_text(json.dumps(results, ensure_ascii=False, indent=2) + "\n")
        intact = all(results[k] for k in ["module_cache_unchanged", "contract_unchanged", "core_module_inputs_unchanged", "fixture_tests_unchanged"])
        print(json.dumps({"success": results.get("success", False), "inputs_intact": intact}), flush=True)
        return 0 if results.get("success") and intact else 1


if __name__ == "__main__":
    raise SystemExit(main())
