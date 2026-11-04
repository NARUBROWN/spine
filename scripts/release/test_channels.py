"""채널 혼합, 미완료 코어 후보 승격과 코드가 섞인 의존성 승격을 거부합니다."""

import copy
import json
import os
from pathlib import Path
import tempfile
import subprocess
import unittest
from unittest.mock import patch

from test_release import FakeGitHub, POLICY, SOURCE, NOW
import release
import pin_channel
import verify_public
import configure_branches
import ci_workspace


def core_policy():
    result = copy.deepcopy(POLICY)
    result.update(kind="core", module=release.CORE, next_version="v0.7.0")
    result.pop("ci_core", None)
    result["ci_sources"] = {name: SOURCE for name in release.PLATFORMS}
    result["channels"] = {
        channel: {"core_version": "v0.7.0" + suffix,
                  "modules": {name: "v0.2.0" + suffix for name in release.PLATFORMS}}
        for channel, suffix in (("preview", "-preview.1"), ("rc", "-rc.1"), ("stable", ""))}
    return result


def platform_policy(name):
    result = core_policy()
    result.update(kind="platform", module=name, next_version="v0.2.0", ci_core=SOURCE)
    result.pop("ci_sources")
    for selected in result["channels"].values():
        selected["modules"] = {}
    return result


class ChannelPolicyTests(unittest.TestCase):
    def test_channel_selection_distinguishes_branches_from_complete_tags(self):
        expected = {"preview": "preview", "rc": "rc", "main": "stable",
                    "v0.7.0-preview.1": "preview", "v0.7.0-rc.1": "rc", "v0.7.0": "stable",
                    "validator-fix": "preview", "version-bump": "preview", "v-next": "preview"}
        for ref, channel in expected.items():
            with self.subTest(ref=ref):
                self.assertEqual(ci_workspace.channel_for(ref), channel)

    def test_initial_channel_creation_is_explicit_and_never_moves_existing_branches(self):
        writes = []
        class Repository:
            repository = "NARUBROWN/spine-kafka-platform"
            def api(self, path, **kwargs):
                if kwargs.get("method") == "POST":
                    writes.append(kwargs["data"])
                    return {}
                return {"object": {"sha": SOURCE}} if path.endswith("/main") else None
        configure_branches.bootstrap(Repository())
        self.assertEqual(writes, [])
        configure_branches.bootstrap(Repository(), apply=True)
        self.assertEqual(writes, [{"ref": "refs/heads/preview", "sha": SOURCE}, {"ref": "refs/heads/rc", "sha": SOURCE}])
        with patch.object(Repository, "api", return_value={"object": {"sha": SOURCE}}) as api:
            configure_branches.bootstrap(Repository(), apply=True)
            self.assertTrue(all(call.kwargs.get("method", "GET") == "GET" for call in api.call_args_list))

    def test_every_platform_and_core_channel_is_explicit_and_matching(self):
        for policy in (core_policy(), platform_policy(release.CORE + "-echo-platform")):
            release.validate_policy(policy)
            for channel, selected in policy["channels"].items():
                self.assertEqual(release.parse_tag(selected["core_version"])[1], channel)
            for value in ("v0.7.0", "v0.7.0-rc.1", "main", "v0.7.0-0.20261104000000-aaaaaaaaaaaa"):
                bad = copy.deepcopy(policy)
                bad["channels"]["preview"]["core_version"] = value
                with self.subTest(value=value), self.assertRaises(release.ReleaseError):
                    release.validate_policy(bad)
        bad = core_policy()
        bad["channels"]["preview"]["modules"].pop(next(iter(release.PLATFORMS)))
        with self.assertRaises(release.ReleaseError):
            release.validate_policy(bad)

    def test_promotion_normalizes_only_managed_channel_metadata(self):
        before = b"module example.com/app\n\ngo 1.26.0\nrequire (\n github.com/NARUBROWN/spine v0.7.0-preview.1\n example.com/native v1.2.3\n)\n"
        after = before.replace(b"v0.7.0-preview.1", b"v0.7.0-rc.2")
        self.assertEqual(release.normalized_file("go.mod", before), release.normalized_file("go.mod", after))
        for changed in (after.replace(b"v0.7.0-rc.2", b"v0.8.0-rc.1"), after.replace(b"v1.2.3", b"v1.2.4"), after+b"replace example.com/native => ../local\n"):
            self.assertNotEqual(release.normalized_file("go.mod", before), release.normalized_file("go.mod", changed))
        self.assertNotEqual(release.normalized_file("main.go", b"package first"), release.normalized_file("main.go", b"package second"))
        single = b"module example.com/app\nrequire github.com/NARUBROWN/spine v0.7.0-preview.1\n"
        self.assertEqual(release.normalized_file("go.mod", single), release.normalized_file("go.mod", single.replace(b"preview.1", b"rc.1")))
        excluded = b"exclude (\n github.com/NARUBROWN/spine v0.7.0-preview.1\n)\n"
        self.assertNotEqual(release.normalized_file("go.mod", excluded), release.normalized_file("go.mod", excluded.replace(b"preview.1", b"rc.1")))
        original = core_policy()
        updated = copy.deepcopy(original)
        updated["ci_sources"] = {name: "b" * 40 for name in release.PLATFORMS}
        key = ".github/release-policy.json"
        self.assertEqual(release.normalized_file(key, json.dumps(original).encode()), release.normalized_file(key, json.dumps(updated).encode()))
        updated["preview_hours"] += 1
        self.assertNotEqual(release.normalized_file(key, json.dumps(original).encode()), release.normalized_file(key, json.dumps(updated).encode()))

    def test_module_pin_planning_has_no_writes_until_applied(self):
        policy = platform_policy(release.CORE + "-echo-platform")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / ".github").mkdir()
            (root / ".github/release-policy.json").write_text(json.dumps(policy))
            mod = root / "go.mod"
            original = "module " + policy["module"] + "\n\ngo 1.26.0\n\nrequire github.com/NARUBROWN/spine v0.7.0-preview.1\n"
            mod.write_text(original)
            pin_channel.pin(root, "rc")
            self.assertEqual(mod.read_text(), original)
            pin_channel.pin(root, "rc", apply=True)
            release.check_module(policy, "rc", root)
            with self.assertRaises(release.ReleaseError):
                release.check_module(policy, "preview", root)

    def test_source_workspace_builds_without_querying_unpublished_tags(self):
        policy = platform_policy(release.CORE + "-echo-platform")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            (root / ".github").mkdir()
            (root / ".github/release-policy.json").write_text(json.dumps(policy))
            (root / "go.mod").write_text("module " + policy["module"] + "\n\ngo 1.26.0\n\nrequire github.com/NARUBROWN/spine v0.7.0-preview.1\n")
            (root / "source.go").write_text('package platform\nimport spine "github.com/NARUBROWN/spine"\nconst Value = spine.Value\n')
            source = root / "spine-core"
            source.mkdir()
            (source / "go.mod").write_text("module github.com/NARUBROWN/spine\n\ngo 1.26.0\n")
            (source / "source.go").write_text("package spine\nconst Value = 1\n")
            with patch.dict(os.environ, {"GITHUB_ENV": ""}):
                workspace = ci_workspace.prepare(root, root / "source.work")
            result = subprocess.run(["go", "build", "./..."], cwd=root,
                                    env=dict(os.environ, GOWORK=str(workspace), GOPROXY="off", GOSUMDB="off"),
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr + "\nroot=" + str(root) + "\n" + workspace.read_text())
            self.assertNotIn("replace", (root / "go.mod").read_text())

    def test_dependency_free_core_and_empty_child_module_require_lists_are_valid(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "go.mod").write_text("module " + release.CORE + "\n\ngo 1.26.0\n")
            for name in ("test", "test/openapi", "test/platform-e2e", "cmd/demo", "cmd/reference", "examples/security-config"):
                child = root / name
                child.mkdir(parents=True, exist_ok=True)
                (child / "go.mod").write_text("module example.com/child\n\ngo 1.26.0\n")
            release.check_module(core_policy(), "preview", root)

    def test_core_only_verification_does_not_checkout_release_examples_or_platforms(self):
        policy = core_policy()
        plan = {"kind": "core", "module": release.CORE, "components": release.components(policy), "source_sha": SOURCE}
        with tempfile.TemporaryDirectory() as directory:
            with patch.object(verify_public, "consumer", return_value={release.CORE: "v0.7.0-preview.1"}), \
                    patch.object(verify_public.subprocess, "check_output", return_value=b'{"Version":"v0.7.0-preview.1"}'), \
                    patch.object(verify_public, "checkout_example", side_effect=AssertionError("예제는 정식 발행 후 확인합니다")), \
                    patch.object(verify_public, "run", return_value="go version"):
                report = verify_public.verify(plan, Path(directory), Path(directory), {}, "v0.7.0-preview.1", "core")
                self.assertEqual(report["scope"], "core")

    def test_core_candidate_scope_never_counts_as_a_completed_candidate(self):
        github = FakeGitHub()
        tag = github.candidate()
        github.manifests[tag]["validation"]["scope"] = "core"
        with self.assertRaises(release.ReleaseError):
            release.make_plan(github, POLICY, "rc", POLICY["next_version"] + "-rc.1", tag, now=NOW)

    def test_promotion_rejects_missing_or_different_selected_modules(self):
        for replacement in (None, "v0.6.5", "v0.7.0-rc.1"):
            github = FakeGitHub()
            source = github.candidate()
            selected = github.manifests[source]["validation"]["selected_modules"]
            if replacement is None:
                selected.pop(release.CORE)
            else:
                selected[release.CORE] = replacement
            with self.subTest(replacement=replacement), self.assertRaises(release.ReleaseError):
                release.make_plan(github, POLICY, "rc", POLICY["next_version"] + "-rc.1", source, now=NOW)

    def test_source_sha_consumer_report_cannot_complete_a_tagged_candidate(self):
        plan = {"kind": "core", "module": release.CORE, "version": "v0.7.0-preview.1",
                "components": release.components(core_policy())}
        report = {"selected_modules": {release.CORE: "v0.7.0-0.20261104000000-aaaaaaaaaaaa"}}
        self.assertFalse(release.selected_modules_match(plan, report, "core"))
        report["selected_modules"][release.CORE] = plan["version"]
        self.assertTrue(release.selected_modules_match(plan, report, "core"))

    def test_metadata_promotion_branch_must_descend_from_verified_source(self):
        github = FakeGitHub()
        source = github.candidate()
        version = POLICY["next_version"] + "-rc.1"
        github.promotion_head = "f" * 40
        github.pull_direction = ("promote/rc/" + version, "rc")
        plan = release.make_plan(github, POLICY, "rc", version, source, now=NOW)
        self.assertEqual(plan["promotion_pr"]["head_sha"], "f" * 40)
        github.comparison = "diverged"
        with self.assertRaises(release.ReleaseError):
            release.make_plan(github, POLICY, "rc", version, source, now=NOW)


class CohortTests(unittest.TestCase):
    def setUp(self):
        self.policy = core_policy()
        self.github = FakeGitHub()
        self.github.repository = "NARUBROWN/spine"
        self.github.policy = self.policy
        self.version = self.policy["channels"]["preview"]["core_version"]
        self.candidate = {"source_sha": SOURCE, "components": release.components(self.policy), "kind": "core"}
        self.peers = {}
        for name, tag in self.policy["channels"]["preview"]["modules"].items():
            peer = FakeGitHub()
            peer.repository = name.removeprefix("github.com/")
            peer.policy = platform_policy(name)
            peer.tags[tag] = "b" * 40
            self.peers[peer.repository] = peer
        self.core_version = self.version

    def verify(self, github, tag, channel, policy, bootstrap=False):
        if bootstrap:
            return {"id": 1, "digest": "sha256:core"}, self.candidate
        return {"id": 2, "digest": "sha256:platform"}, {"kind": "platform", "components": {"core_version": self.core_version}}

    def complete(self):
        with patch.dict(os.environ, {"GITHUB_REF": "refs/heads/preview"}), patch.object(release, "verify_source_candidate", side_effect=self.verify):
            return release.complete_plan(self.github, self.policy, self.version, self.peers.__getitem__)

    def test_completion_requires_all_five_exact_platform_manifests(self):
        plan = self.complete()
        self.assertEqual(len(plan["platform_evidence"]), 5)
        peer = next(iter(self.peers.values()))
        peer.tags.clear()
        with self.assertRaises(release.ReleaseError):
            self.complete()

    def test_platform_using_another_core_cannot_complete_the_cohort(self):
        self.core_version = "v0.7.0-preview.2"
        with self.assertRaises(release.ReleaseError):
            self.complete()

    def test_moving_core_head_after_candidate_publication_blocks_completion(self):
        self.github.heads["preview"] = "c" * 40
        with self.assertRaises(release.ReleaseError):
            self.complete()


if __name__ == "__main__":
    unittest.main()
