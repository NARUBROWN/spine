"""Core candidate is the single source for platform dependency updates."""

import copy
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import release
import sync_platforms
import sync_platform_prs
from test_release import POLICY, SOURCE


class SyncPlatformsTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.parent = Path(self.temp.name)
        self.modules = [release.CORE + "-echo-platform", release.CORE + "-nats-platform"]
        self.selected = {"core_version": "v0.7.0-preview.12", "modules": {
            self.modules[0]: "v0.2.0-preview.5", self.modules[1]: "v0.1.0-preview.2"}}
        original = Path(__file__).parent
        for module in self.modules:
            root = self.parent / module.removeprefix("github.com/NARUBROWN/")
            (root / ".github").mkdir(parents=True)
            shutil.copytree(original, root / "scripts/release")
            policy = copy.deepcopy(POLICY)
            policy.update(kind="platform", module=module, ci_core=SOURCE,
                          next_version="v0.1.0" if module.endswith("nats-platform") else "v0.2.0")
            policy.pop("ci_sources", None)
            policy["channels"] = {name: {"core_version": "v0.7.0" + suffix, "modules": {}}
                                  for name, suffix in (("preview", "-preview.2"), ("rc", "-rc.1"), ("stable", ""))}
            (root / ".github/release-policy.json").write_text(json.dumps(policy))
            (root / "go.mod").write_text(f"module {module}\n\ngo 1.26.0\n\nrequire {release.CORE} v0.7.0-preview.2\n")
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            subprocess.run(["git", "add", "."], cwd=root, check=True)
            subprocess.run(["git", "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "fixture"], cwd=root, check=True)

    def test_dry_run_and_apply_use_verified_combination(self):
        with patch.object(sync_platforms, "candidate", return_value=(SOURCE, "preview", self.selected)):
            sync_platforms.synchronize("v0.7.0-preview.12", self.parent, tidy=False)
            for module in self.modules:
                root = self.parent / module.removeprefix("github.com/NARUBROWN/")
                self.assertIn("v0.7.0-preview.2", (root / "go.mod").read_text())
            sync_platforms.synchronize("v0.7.0-preview.12", self.parent, apply=True, tidy=False)
        for module in self.modules:
            root = self.parent / module.removeprefix("github.com/NARUBROWN/")
            policy = json.loads((root / ".github/release-policy.json").read_text())
            self.assertEqual(policy["channels"]["preview"]["core_version"], "v0.7.0-preview.12")
            self.assertEqual(policy["ci_core"], SOURCE)
            self.assertIn("v0.7.0-preview.12", (root / "go.mod").read_text())

    def test_dirty_checkout_aborts_all_before_writing(self):
        root = self.parent / "spine-nats-platform"
        (root / "untracked").write_text("keep")
        with patch.object(sync_platforms, "candidate", return_value=(SOURCE, "preview", self.selected)):
            with self.assertRaises(release.ReleaseError):
                sync_platforms.synchronize("v0.7.0-preview.12", self.parent, apply=True, tidy=False)
        self.assertIn("v0.7.0-preview.2", (self.parent / "spine-echo-platform/go.mod").read_text())
        self.assertEqual((root / "untracked").read_text(), "keep")

    def test_candidate_change_aborts_before_writing(self):
        changed = copy.deepcopy(self.selected)
        changed["modules"][self.modules[1]] = "v0.1.0-preview.3"
        with patch.object(sync_platforms, "candidate", side_effect=[(SOURCE, "preview", self.selected), (SOURCE, "preview", changed)]):
            with self.assertRaises(release.ReleaseError):
                sync_platforms.synchronize("v0.7.0-preview.12", self.parent, apply=True, tidy=False)
        self.assertIn("v0.7.0-preview.2", (self.parent / "spine-echo-platform/go.mod").read_text())

    def test_preview_opens_platform_prs_on_separate_branches(self):
        remotes = self.parent / "remotes"
        remotes.mkdir()
        for module in self.modules:
            name = module.removeprefix("github.com/NARUBROWN/")
            source = self.parent / name
            subprocess.run(["git", "branch", "preview"], cwd=source, check=True)
            subprocess.run(["git", "clone", "-q", "--bare", str(source), str(remotes / (name + ".git"))], check=True)
        destination = self.parent / "clones"
        destination.mkdir()
        native = sync_platform_prs.run
        opened = []
        def fake_run(*args, cwd=None):
            if args[:3] == ("gh", "repo", "clone"):
                name = args[3].split("/")[-1]
                subprocess.run(["git", "clone", "-q", "--branch", "preview", str(remotes / (name + ".git")), args[4]], check=True)
                return ""
            if args[:3] == ("gh", "pr", "create"):
                opened.append(args)
                return "https://example.invalid/pr"
            return native(*args, cwd=cwd)
        def fake_sync(tag, workspace, apply=False):
            for module in self.modules:
                root = workspace / module.removeprefix("github.com/NARUBROWN/")
                with (root / "go.mod").open("a") as stream:
                    stream.write("\n// synchronized\n")
            return SOURCE
        with patch.object(sync_platform_prs, "candidate", return_value=(SOURCE, "preview", self.selected)), \
                patch.object(sync_platform_prs, "run", side_effect=fake_run), \
                patch.object(sync_platform_prs, "synchronize", side_effect=fake_sync):
            sync_platform_prs.open_prs("v0.7.0-preview.12", destination)
        self.assertEqual(len(opened), 2)
        for args in opened:
            self.assertEqual(args[args.index("--base") + 1], "preview")
            self.assertEqual(args[args.index("--head") + 1], "sync/core-v0.7.0-preview.12")
        for module in self.modules:
            name = module.removeprefix("github.com/NARUBROWN/")
            result = subprocess.run(["git", "--git-dir", str(remotes / (name + ".git")), "show-ref", "--verify", "refs/heads/sync/core-v0.7.0-preview.12"], capture_output=True)
            self.assertEqual(result.returncode, 0)

    def test_existing_remote_branch_aborts_before_any_sync(self):
        with patch.object(sync_platform_prs, "candidate", return_value=(SOURCE, "preview", self.selected)), \
                patch.object(sync_platform_prs, "run", return_value=""), \
                patch.object(sync_platform_prs.subprocess, "run", return_value=SimpleNamespace(returncode=0, stderr="")), \
                patch.object(sync_platform_prs, "synchronize") as synchronize:
            with self.assertRaisesRegex(release.ReleaseError, "already exists"):
                sync_platform_prs.open_prs("v0.7.0-preview.12", self.parent)
            synchronize.assert_not_called()

    def test_promotion_requires_previous_complete_platform_evidence(self):
        with patch.object(sync_platform_prs, "GitHub") as factory:
            github = factory.return_value
            github.tag_sha.return_value = SOURCE
            policy = copy.deepcopy(POLICY)
            policy.update(kind="core", module=release.CORE, next_version="v0.7.0")
            policy.pop("ci_core", None)
            policy["ci_sources"] = {name: SOURCE for name in release.PLATFORMS}
            policy["channels"] = {name: {"core_version": "v0.7.0" + suffix,
                                  "modules": {module: "v0.2.0" + suffix for module in release.PLATFORMS}}
                                  for name, suffix in (("preview", "-preview.1"), ("rc", "-rc.1"), ("stable", ""))}
            import base64
            github.api.return_value = {"content": base64.b64encode(json.dumps(policy).encode()).decode()}
            with patch.object(sync_platform_prs, "verify_source_candidate", side_effect=[
                    ({}, {"source_tag": "v0.7.0-preview.1"}), ({}, {"platform_evidence": {}})]):
                with self.assertRaisesRegex(release.ReleaseError, "lacks verified"):
                    sync_platform_prs.promotion_sources("v0.7.0-rc.1", self.selected, github)

    def test_promotion_uses_verified_previous_platform_tags(self):
        import base64
        policy = copy.deepcopy(POLICY)
        policy.update(kind="core", module=release.CORE, next_version="v0.7.0")
        policy.pop("ci_core", None)
        policy["ci_sources"] = {name: SOURCE for name in release.PLATFORMS}
        policy["channels"] = {name: {"core_version": "v0.7.0" + suffix,
                              "modules": {module: "v0.2.0" + suffix for module in release.PLATFORMS}}
                              for name, suffix in (("preview", "-preview.1"), ("rc", "-rc.1"), ("stable", ""))}
        evidence = {module: {"version": "v0.2.0-preview.1", "source_sha": SOURCE}
                    for module in self.selected["modules"]}
        previous = {"platform_evidence": evidence, "components": {"modules": {
                    module: item["version"] for module, item in evidence.items()}}}
        with patch.object(sync_platform_prs, "GitHub") as factory:
            github = factory.return_value
            github.tag_sha.return_value = SOURCE
            github.api.return_value = {"content": base64.b64encode(json.dumps(policy).encode()).decode()}
            with patch.object(sync_platform_prs, "verify_source_candidate", side_effect=[
                    ({}, {"source_tag": "v0.7.0-preview.1"}), ({}, previous)]):
                self.assertEqual(sync_platform_prs.promotion_sources("v0.7.0-rc.1", self.selected, github), evidence)
            previous["platform_evidence"][self.modules[0]]["version"] = "v0.2.0-rc.1"
            with patch.object(sync_platform_prs, "verify_source_candidate", side_effect=[
                    ({}, {"source_tag": "v0.7.0-preview.1"}), ({}, previous)]):
                with self.assertRaises(release.ReleaseError):
                    sync_platform_prs.promotion_sources("v0.7.0-rc.1", self.selected, github)

    def test_platform_version_mismatch_is_rejected(self):
        changed = copy.deepcopy(self.selected)
        changed["modules"][self.modules[1]] = "v0.2.0-preview.2"
        with patch.object(sync_platforms, "candidate", return_value=(SOURCE, "preview", changed)):
            with self.assertRaises(release.ReleaseError):
                sync_platforms.synchronize("v0.7.0-preview.12", self.parent, apply=True, tidy=False)


if __name__ == "__main__":
    unittest.main()
