#!/usr/bin/env python3
"""후보 태그가 모두 생기기 전의 소스 CI와 공개 조합 검증을 구분합니다."""

import argparse
import json
import os
from pathlib import Path
import subprocess

from release import CORE, PLATFORMS, ReleaseError, SHA, TAG, parse_tag, validate_policy


def channel_for(ref):
    if ref in ("preview", "rc", "main"):
        return "stable" if ref == "main" else ref
    if TAG.fullmatch(ref):
        return parse_tag(ref)[1]
    return "preview"


def prepare(root, output, published=False):
    root = root.resolve()
    policy = json.loads((root / ".github/release-policy.json").read_text(encoding="utf-8"))
    validate_policy(policy)
    if policy["kind"] == "core":
        members = [root, *[root.parent / name.removeprefix("github.com/NARUBROWN/") for name in sorted(PLATFORMS)]]
        members += [root / name for name in ("test", "test/openapi", "test/platform-e2e", "cmd/demo", "cmd/reference", "examples/security-config")]
    else:
        members = [root, root / "spine-core"]
    for member in members:
        if not (member / "go.mod").is_file():
            raise ReleaseError("소스 CI 모듈이 없습니다: " + str(member))
    output = output.resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    text = "go 1.26.0\n\nuse (\n" + "".join("\t" + json.dumps(str(path.resolve())) + "\n" for path in members) + ")\n"
    if not published:
        directories = {CORE: root if policy["kind"] == "core" else root / "spine-core"}
        if policy["kind"] == "core":
            directories.update({name: root.parent / name.removeprefix("github.com/NARUBROWN/") for name in PLATFORMS})
        # Go의 의존 그래프도 아직 없는 태그를 조회하지 않도록 버전별로 연결합니다.
        text += "\nreplace (\n"
        for selected in policy["channels"].values():
            for module, version in {CORE: selected["core_version"], **selected["modules"]}.items():
                text += f"\t{module} {version} => {json.dumps(str(directories[module].resolve()))}\n"
        text += ")\n"
    output.write_text(text)
    if target := os.environ.get("GITHUB_ENV"):
        with open(target, "a") as stream:
            stream.write("GOWORK=" + str(output) + "\n")
    return output


def check(root):
    policy = json.loads((root / ".github/release-policy.json").read_text(encoding="utf-8"))
    members = [root]
    if policy["kind"] == "core":
        members += [root.parent / name.removeprefix("github.com/NARUBROWN/") for name in sorted(PLATFORMS)]
    for member in members:
        parsed = json.loads(subprocess.check_output(["go", "mod", "edit", "-json"], cwd=member, text=True))
        if parsed.get("Replace"):
            raise ReleaseError("소스 CI에서도 플랫폼과 코어 본체에 replace를 커밋하지 않습니다")
        subprocess.run(["go", "mod", "verify"], cwd=member, check=True)
    subprocess.run(["git", "diff", "--exit-code", "--", "go.mod", "go.sum", ":(glob)**/go.mod", ":(glob)**/go.sum"], cwd=root, check=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("refs", "prepare", "check"))
    parser.add_argument("--root", type=Path, default=Path("."))
    parser.add_argument("--output", type=Path)
    parser.add_argument("--published", action="store_true")
    parser.add_argument("--ref", default="preview")
    args = parser.parse_args()
    root = args.root.resolve()
    if args.command == "prepare":
        if args.output is None:
            parser.error("prepare에는 --output이 필요합니다")
        prepare(root, args.output, args.published)
    elif args.command == "check":
        check(root)
    else:
        policy = json.loads((root / ".github/release-policy.json").read_text(encoding="utf-8"))
        validate_policy(policy)
        selected = policy["channels"][channel_for(args.ref)]["modules"] if args.published else policy["ci_sources"]
        with open(os.environ["GITHUB_OUTPUT"], "a") as stream:
            for module, ref in selected.items():
                name = module.removeprefix(CORE + "-").removesuffix("-platform").replace("validator-playground", "playground")
                stream.write(name + "=" + ref + "\n")


if __name__ == "__main__":
    main()
