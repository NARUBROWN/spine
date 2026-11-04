#!/usr/bin/env python3
"""채널 승격에 필요한 공식 모듈 버전만 계획하거나 로컬 파일에 적용합니다."""

import argparse
import difflib
import json
import os
from pathlib import Path
import subprocess
import tempfile

from release import CORE, components, validate_policy


def pin(root, channel, apply=False, tidy=False):
    policy = json.loads((root / ".github/release-policy.json").read_text())
    validate_policy(policy)
    selected = components(policy, channel)
    expected = {CORE: selected["core_version"], **selected["modules"]}
    directories = [root]
    if policy["kind"] == "core":
        directories += [root / path for path in ("test", "test/openapi", "test/platform-e2e", "cmd/demo", "cmd/reference", "examples/security-config")]
    env = dict(os.environ, GOWORK="off")
    for directory in directories:
        path = directory / "go.mod"
        original = path.read_text()
        parsed = json.loads(subprocess.check_output(["go", "mod", "edit", "-json"], cwd=directory, env=env, text=True))
        required = {item["Path"] for item in (parsed.get("Require") or [])}
        with tempfile.TemporaryDirectory(prefix="spine-channel-pins-") as temporary:
            modfile = Path(temporary) / "go.mod"
            modfile.write_text(original)
            for module, version in expected.items():
                if module in required:
                    subprocess.run(["go", "mod", "edit", "-modfile=" + str(modfile), "-require=" + module + "@" + version], cwd=directory, env=env, check=True)
            updated = modfile.read_text()
        if updated != original:
            print("".join(difflib.unified_diff(original.splitlines(True), updated.splitlines(True), fromfile=str(path), tofile=str(path))), end="")
            if apply:
                path.write_text(updated)
        if apply and tidy:
            # 코어 후보 태그를 공개한 다음 실행해 실제 공개 의존성의 체크섬을 기록합니다.
            subprocess.run(["go", "mod", "tidy"], cwd=directory, env=env, check=True)
            subprocess.run(["go", "mod", "verify"], cwd=directory, env=env, check=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path("."))
    parser.add_argument("--channel", choices=("preview", "rc", "stable"), required=True)
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--tidy", action="store_true")
    args = parser.parse_args()
    if args.tidy and not args.apply:
        parser.error("--tidy는 --apply와 함께 사용하세요")
    pin(args.root.resolve(), args.channel, args.apply, args.tidy)
