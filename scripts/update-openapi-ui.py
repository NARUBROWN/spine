#!/usr/bin/env python3
"""임베드 UI 배포본, 무결성과 재배포 고지를 함께 관리합니다."""

import argparse
import base64
from concurrent.futures import ThreadPoolExecutor
import hashlib
import io
import json
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile
import urllib.request


ROOT = Path(__file__).resolve().parents[1]
TOOLS = ROOT / "scripts/openapi-ui"
MANIFEST = TOOLS / "manifest.json"
REGISTRY = "https://registry.npmjs.org/"
SUPPORTED = {"swagger-ui-dist": 5, "redoc": 2}
ASSETS = {
    "swagger-ui-dist": ("swaggerui", ["swagger-ui-bundle.js", "swagger-ui.css", "LICENSE", "NOTICE", "swagger-ui-bundle.js.LICENSE.txt"]),
    "redoc": ("redoc", ["bundles/redoc.standalone.js", "LICENSE"]),
}
# 다중 라이선스는 허용되는 선택지가 있는 경우에만 처리합니다.
PERMITTED = {"MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "ISC", "0BSD", "CC0-1.0", "Unlicense", "BlueOak-1.0.0", "CC-BY-4.0", "Python-2.0"}


def download(url):
    if not url.startswith(REGISTRY):
        raise ValueError(f"Distribution source is not allowed: {url}")
    request = urllib.request.Request(url, headers={"User-Agent": "spine-openapi-ui-updater"})
    with urllib.request.urlopen(request, timeout=60) as response:
        if not response.url.startswith(REGISTRY):
            raise ValueError("Distribution source changed")
        return response.read()


def metadata(name):
    return json.loads(download(REGISTRY + name))


def stable(version):
    match = re.fullmatch(r"(\d+)\.(\d+)\.(\d+)", version)
    return tuple(map(int, match.groups())) if match else None


def upstream():
    pinned = json.loads((TOOLS / "package.json").read_text())["dependencies"]
    report = {}
    for name, major in SUPPORTED.items():
        package = metadata(name)
        available = [v for v in package["versions"] if stable(v) and stable(v)[0] == major]
        version = max(available, key=stable)
        report[name] = {"pinned": pinned[name], "supported": version, "latest": package["dist-tags"]["latest"], "update_available": version != pinned[name]}
    return report


def permitted(expression):
    if not isinstance(expression, str):
        return False
    tokens = re.findall(r"\(|\)|[^\s()]+", expression)
    position = 0

    def atom():
        nonlocal position
        token = tokens[position]
        position += 1
        if token == "(":
            value = disjunction()
            if tokens[position] != ")": raise ValueError("invalid SPDX expression")
            position += 1
            return value
        if token in {"AND", "OR", ")"}: raise ValueError("invalid SPDX expression")
        return token in PERMITTED

    def conjunction():
        nonlocal position
        value = atom()
        while position < len(tokens) and tokens[position] == "AND":
            position += 1
            other = atom()
            value = value and other
        return value

    def disjunction():
        nonlocal position
        value = conjunction()
        while position < len(tokens) and tokens[position] == "OR":
            position += 1
            other = conjunction()
            value = value or other
        return value

    try:
        result = disjunction()
        return position == len(tokens) and result
    except (ValueError, IndexError):
        return False


def archive(entry):
    integrity = entry["integrity"]
    cache = Path(tempfile.gettempdir()) / "spine-openapi-ui-cache"
    cache.mkdir(exist_ok=True)
    cached = cache / hashlib.sha256(integrity.encode()).hexdigest()
    data = cached.read_bytes() if cached.exists() else download(entry["resolved"])
    algorithm, expected = integrity.split("-", 1)
    if algorithm != "sha512" or base64.b64encode(hashlib.sha512(data).digest()).decode() != expected:
        raise ValueError(f"Distribution integrity verification failed: {entry['resolved']}")
    cached.write_bytes(data)
    result = {}
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as bundle:
        for member in bundle.getmembers():
            if member.isfile() and "/" in member.name:
                relative = member.name.split("/", 1)[1]
                # 경로에 쓰지 않고 필요한 파일만 메모리로 읽습니다.
                if "/" not in relative or relative.startswith("bundles/") or re.search(r"(?:^|/)(?:licen[cs]e|copying|copyright|notice)(?:\.|$|-)", relative, re.I):
                    result[relative] = bundle.extractfile(member).read()
    return result


def dependency_notices(item):
    location, entry = item
    name = location.rsplit("node_modules/", 1)[-1]
    contents = archive(entry)
    package = json.loads(contents["package.json"])
    expression = entry.get("license") or package.get("license") or " OR ".join(item["type"] for item in package.get("licenses", []))
    if isinstance(expression, dict): expression = expression.get("type", "")
    reviewed = {
        "decko@1.2.0": ("MIT", "d20ef4c192faa3c560bc50bdd780f686804e355957d4146ddf8cbb58eb2e3dba"),
        "stickyfill@1.1.1": ("MIT", "82183d79743926de3202987112039cda376a447a07422d45de1392cd85e40d07"),
        "url-template@2.0.8": ("BSD-3-Clause", "70723b90e3f26aa2808616e846cd8cd349fe33864fd328a91a5bb9d33e58e9d9"),
    }
    identity = name + "@" + entry["version"]
    if identity in reviewed:
        approved, digest = reviewed[identity]
        if hashlib.sha256(contents.get("LICENSE", b"")).hexdigest() != digest:
            raise ValueError(f"Reviewed license text changed: {identity}")
        expression = approved
    if not permitted(expression):
        raise ValueError(f"License review required: {name}@{entry['version']} ({expression})")
    notices = [(key, value) for key, value in contents.items() if re.search(r"(?:^|/)(?:licen[cs]e|copying|copyright|notice)(?:\.|$|-)", key, re.I)]
    if not notices:
        overrides = json.loads((TOOLS / "license-overrides.json").read_text())
        if identity in overrides:
            override = overrides[identity]
            data = (TOOLS / override["file"]).read_bytes()
            if override["license"] != expression or hashlib.sha256(data).hexdigest() != override["sha256"]:
                raise ValueError(f"Supplementary license text changed: {identity}")
            notices.append((override["source"], data))
        else:
            # 배포본에 별도 원문이 없으면 SPDX 선언, 원래 저자와 README의
            # 저작권 고지를 그대로 보관하고 동일 라이선스의 표준 원문을 덧붙입니다.
            canonical = TOOLS / "licenses" / (expression + ".txt")
            if expression not in {"MIT", "Apache-2.0", "CC0-1.0"} or not canonical.exists():
                raise ValueError(f"Redistribution notice review required: {identity}")
            notices.append(("original package.json", contents["package.json"]))
            notices.extend((key, value) for key, value in contents.items() if key.lower().startswith("readme"))
            notices.append((expression + " license terms", canonical.read_bytes()))
    text = f"\n{'=' * 72}\n{name}@{entry['version']}\nLicense: {expression}\nSource: {entry['resolved']}\n"
    for filename, data in sorted(notices):
        text += f"\n--- {filename} ---\n" + data.decode("utf-8", errors="strict") + "\n"
    return text, {"name": name, "version": entry["version"], "license": expression, "integrity": entry["integrity"]}


def patch_swagger_bundle(data, version):
    patches = json.loads((TOOLS / "swagger-ui-fixes.json").read_text())
    if version != patches["version"]:
        raise ValueError("Local Swagger UI fixes require review for version " + version)
    if hashlib.sha256(data).hexdigest() != patches["upstream_sha256"]:
        raise ValueError("Local Swagger UI fixes require the reviewed upstream bundle")
    for patch in patches["replacements"]:
        before, after = patch["before"].encode(), patch["after"].encode()
        if data.count(before) != 1:
            raise ValueError("Local Swagger UI fix no longer matches: " + patch["issue"])
        data = data.replace(before, after)
    return patches["notice"].encode() + data


def refresh(update):
    package_file = TOOLS / "package.json"
    package = json.loads(package_file.read_text())
    if update:
        for name, info in upstream().items():
            package["dependencies"][name] = info["supported"]
            if name == "swagger-ui-dist":
                package["dependencies"]["swagger-ui"] = info["supported"]
    # 준비가 끝나기 전에는 저장소 파일을 바꾸지 않습니다.
    with tempfile.TemporaryDirectory(prefix="spine-ui-update-") as directory:
        staging = Path(directory)
        (staging / "package.json").write_text(json.dumps(package, indent=2) + "\n")
        lock_path = TOOLS / "package-lock.json"
        if lock_path.exists():
            (staging / "package-lock.json").write_bytes(lock_path.read_bytes())
        subprocess.run(["npm", "install", "--package-lock-only", "--ignore-scripts", "--omit=dev", "--no-audit", "--no-fund"], cwd=staging, check=True)
        lock = json.loads((staging / "package-lock.json").read_text())
        entries = [(key, value) for key, value in lock["packages"].items() if key and not value.get("dev")]
        print(f"Checking dependency licenses: {len(entries)} packages", flush=True)
        with ThreadPoolExecutor(max_workers=12) as pool:
            futures = [pool.submit(dependency_notices, item) for item in sorted(entries)]
            notices, failures = [], []
            for future in futures:
                try:
                    notices.append(future.result())
                except (ValueError, OSError) as error:
                    failures.append(str(error))
            if failures:
                raise ValueError("\n".join(failures))
        if MANIFEST.exists():
            previous = json.loads(MANIFEST.read_text())
            known = {}
            for dependency in previous["dependencies"]:
                known.setdefault(dependency["name"], set()).add(dependency["license"])
            for _, dependency in notices:
                if dependency["name"] in known and dependency["license"] not in known[dependency["name"]]:
                    raise ValueError(f"License change review required: {dependency['name']} ({dependency['license']})")
        supplementary = "Supplementary runtime dependency license inventory.\nResolved from the pinned source packages. This inventory is a conservative\nsuperset, not an exact bill of materials for the prebuilt JavaScript bundles.\nOriginal bundle notices and license comments are preserved separately.\n" + "".join(text for text, _ in notices)
        files = {}
        distributions = {}
        for name, (target, wanted) in ASSETS.items():
            entry = lock["packages"]["node_modules/" + name]
            expected_license = "Apache-2.0" if name == "swagger-ui-dist" else "MIT"
            if entry.get("license") != expected_license:
                raise ValueError(f"Upstream license changed: {name}")
            contents = archive(entry)
            if name == "swagger-ui-dist":
                contents["swagger-ui-bundle.js"] = patch_swagger_bundle(contents["swagger-ui-bundle.js"], entry["version"])
            if name == "redoc" and contents["bundles/redoc.standalone.js"].count(b"https://cdn.redoc.ly/redoc/logo-mini.svg") != 1:
                raise ValueError("The offline handling of the ReDoc logo requires review")
            exact_notices = []
            # 번들에 붙은 별도 고지 파일도 모두 보존합니다.
            for source in list(wanted) + [p for p in contents if p.endswith(".LICENSE.txt") and ("redoc.standalone" in p)]:
                if source not in contents:
                    raise ValueError(f"Distribution file is missing: {name}/{source}")
                destination = f"pkg/openapi/ui/{target}/assets/{Path(source).name}"
                files[destination] = contents[source]
                if source.endswith(".LICENSE.txt"):
                    exact_notices.append(contents[source].decode())
            files[f"pkg/openapi/ui/{target}/assets/THIRD_PARTY_LICENSES.txt"] = ("Original upstream bundle notices:\n" + "\n".join(exact_notices) + "\n" + supplementary).encode()
            distributions[name] = {"version": entry["version"], "license": entry["license"], "tarball": entry["resolved"], "integrity": entry["integrity"]}
        files["scripts/openapi-ui/package.json"] = (staging / "package.json").read_bytes()
        files["scripts/openapi-ui/package-lock.json"] = (staging / "package-lock.json").read_bytes()
        for input_file in [TOOLS / "license-overrides.json", TOOLS / "swagger-ui-fixes.json", *sorted((TOOLS / "licenses").glob("*.txt"))]:
            files[str(input_file.relative_to(ROOT))] = input_file.read_bytes()
        manifest = {"format": 1, "distributions": distributions, "dependencies": [info for _, info in notices], "files": {name: hashlib.sha256(data).hexdigest() for name, data in sorted(files.items())}}
        manifest["local_patches"] = {"swagger-ui-dist": "scripts/openapi-ui/swagger-ui-fixes.json"}
        for name, data in files.items():
            destination = ROOT / name
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_bytes(data)
        package_file.write_text(json.dumps(package, indent=2) + "\n")
        lock_path.write_bytes((staging / "package-lock.json").read_bytes())
        MANIFEST.write_text(json.dumps(manifest, indent=2) + "\n")
    check()


def check():
    manifest = json.loads(MANIFEST.read_text())
    pinned = json.loads((TOOLS / "package.json").read_text())["dependencies"]
    for name, entry in manifest["distributions"].items():
        if entry["version"] != pinned[name] or stable(entry["version"])[0] != SUPPORTED[name]:
            raise ValueError(f"Pinned version does not match: {name}")
    for name, expected in manifest["files"].items():
        if hashlib.sha256((ROOT / name).read_bytes()).hexdigest() != expected:
            raise ValueError(f"Embedded file changed: {name}")
    print(f"Verified {len(manifest['files'])} asset, notice and input files.")


def main():
    parser = argparse.ArgumentParser(description='Manage embedded UI distributions, integrity and redistribution notices.')
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--check", action="store_true", help="Verify pinned versions and file hashes without network access")
    group.add_argument("--check-upstream", action="store_true", help="Check supported and latest major versions")
    group.add_argument("--refresh", action="store_true", help="Reproduce pinned distributions and notices")
    group.add_argument("--update", action="store_true", help="Update stable versions within supported major versions")
    args = parser.parse_args()
    if args.check:
        check()
    elif args.check_upstream:
        print(json.dumps(upstream(), indent=2))
    else:
        refresh(args.update)


if __name__ == "__main__":
    main()
