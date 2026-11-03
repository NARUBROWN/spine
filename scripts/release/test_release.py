"""발행 전에 막아야 할 잘못된 승격과 공개 검증 누락을 검사합니다."""

import base64
import copy
import datetime as dt
import json
import os
from pathlib import Path
import tempfile
import subprocess
import unittest
from unittest.mock import patch

import release
import verify_public


NOW = dt.datetime(2026, 11, 10, tzinfo=dt.timezone.utc)
SOURCE = "a" * 40
RC_SOURCE = "b" * 40
MAIN_SOURCE = "c" * 40
TREE = "d" * 40
POLICY = json.loads((Path(__file__).resolve().parents[2] / ".github/release-policy.json").read_text())


class FakeGitHub:
    repository = "NARUBROWN/spine"

    def __init__(self):
        self.tags = {}
        self.release_data = {}
        self.manifests = {}
        self.writes = []
        self.comparison = "ahead"
        self.heads = {"preview": SOURCE, "rc": RC_SOURCE, "main": MAIN_SOURCE}
        self.promotion_head = None
        self.changed_tree = False
        self.pull_direction = None
        self.run_conclusion = "success"
        self.run_path = ".github/workflows/preview.yml"
        self.policy = copy.deepcopy(POLICY)
        self.source_policy = None
        self.reviewers = [POLICY["reviewer_login"]]
        self.bypass = False

    def tag_sha(self, tag):
        return self.tags.get(tag)

    def releases(self):
        return list(self.release_data.values())

    def manifest(self, tag):
        return self.manifests[tag]

    def api(self, path, *, missing=False, method="GET", data=None):
        if method != "GET":
            self.writes.append((path, data))
            return {}
        if path.startswith("git/ref/heads/"):
            return {"object": {"sha": self.heads[path.removeprefix("git/ref/heads/")]}}
        if path.startswith("git/commits/"):
            sha = path.removeprefix("git/commits/")
            parents = [{"sha": "e"*40}]
            if sha in [RC_SOURCE, MAIN_SOURCE]:
                parents.append({"sha": self.promotion_head or (SOURCE if sha == RC_SOURCE else RC_SOURCE)})
            return {"tree": {"sha": "f"*40 if self.changed_tree and sha == RC_SOURCE else TREE}, "parents": parents}
        if path.startswith("commits/") and "/pulls" in path:
            sha = path.split("/")[1]
            head, base = self.pull_direction or (("preview", "rc") if sha == RC_SOURCE else ("rc", "main"))
            return [{"number": 1, "merged_at": NOW.isoformat(), "merge_commit_sha": sha,
                     "head": {"ref": head, "repo": {"full_name": self.repository}}, "base": {"ref": base}}]
        if path.startswith("compare/"):
            return {"status": self.comparison}
        if path.startswith("git/matching-refs/"):
            prefix = path.removeprefix("git/matching-refs/")
            return [{"ref": "refs/" + tag} for tag in ["tags/"+t for t in self.tags] if tag.startswith(prefix)]
        if path.startswith("releases/tags/"):
            return self.release_data[path.removeprefix("releases/tags/")]
        if path.startswith("actions/runs/"):
            return {"conclusion": self.run_conclusion, "status": "completed", "event": "workflow_dispatch", "path": self.run_path, "head_branch": self.run_branch, "head_sha": self.run_sha}
        if path.endswith("deployment-branch-policies"):
            return {"branch_policies": [{"name": "main", "type": "branch"}]}
        if path.startswith("environments/"):
            return {"can_admins_bypass": self.bypass, "deployment_branch_policy": {"custom_branch_policies": True},
                    "protection_rules": [{"type": "required_reviewers", "reviewers": [
                        {"type": "User", "reviewer": {"login": name}} for name in self.reviewers]}]}
        if path.startswith("contents/"):
            policy = self.source_policy if SOURCE in path and self.source_policy is not None else self.policy
            return {"content": base64.b64encode(json.dumps(policy).encode()).decode()}
        raise AssertionError(path)

    def candidate(self, channel="preview", age=73):
        tag = f"{POLICY['next_version']}-{channel}.1"
        sha = SOURCE if channel == "preview" else RC_SOURCE
        self.tags[tag] = sha
        self.release_data[tag] = {"tag_name": tag, "draft": False, "prerelease": True,
                                  "published_at": (NOW-dt.timedelta(hours=age)).isoformat()}
        self.manifests[tag] = {"schema_version": "spine.release-manifest/v1", "channel": channel,
                               "version": tag, "source_sha": sha, "branch": POLICY["branches"][channel], "source_tree": TREE, "workflow_run_id": "1",
                               "components": release.components(POLICY)}
        self.manifests[tag]["validation"] = {"result": "passed", "version": tag, "source_sha": sha,
                                             "components": release.components(POLICY)}
        self.run_path = ".github/workflows/"+release.WORKFLOWS[channel]
        self.run_branch, self.run_sha = POLICY["branches"][channel], sha
        return tag


class ReleasePolicyTests(unittest.TestCase):
    def setUp(self):
        self.github = FakeGitHub()
        self.base = POLICY["next_version"]

    def plan(self, channel="preview", version=None, source=SOURCE, reason=""):
        return release.make_plan(self.github, POLICY, channel, version or self.base+"-preview.1", source, reason, NOW)

    def test_preview_requires_immutable_preview_head(self):
        plan = self.plan()
        self.assertEqual(plan["source_sha"], SOURCE)
        for source in ["main", "HEAD", "--help", SOURCE[:12]]:
            with self.subTest(source=source), self.assertRaises(release.ReleaseError):
                self.plan(source=source)
        self.github.heads["preview"] = "f"*40
        with self.assertRaises(release.ReleaseError):
            self.plan()

    def test_reused_or_lower_prerelease_number_is_rejected(self):
        self.github.tags[self.base+"-preview.3"] = SOURCE
        with self.assertRaises(release.ReleaseError):
            self.plan()
        self.plan(version=self.base+"-preview.4")
        with self.assertRaises(release.ReleaseError):
            self.plan(version=self.base+"-preview.3")

    def test_channel_and_version_cannot_be_confused(self):
        for version in [self.base, self.base+"-rc.1", "v9.9.9-preview.1", self.base+"-preview.01", "v01.2.3-preview.1"]:
            with self.subTest(version=version), self.assertRaises(release.ReleaseError):
                self.plan(version=version)

    def test_old_source_cannot_be_retagged_as_new_release(self):
        self.github.source_policy = dict(POLICY, next_version="v0.6.5")
        with self.assertRaises(release.ReleaseError):
            self.plan()

    def test_rc_preserves_preview_commit_and_version_set(self):
        source = self.github.candidate()
        plan = self.plan("rc", self.base+"-rc.1", source)
        self.assertEqual(plan["source_sha"], RC_SOURCE)
        self.assertEqual(plan["upstream_source_sha"], SOURCE)
        self.github.manifests[source]["components"] = {"wrong": "versions"}
        with self.assertRaises(release.ReleaseError):
            self.plan("rc", self.base+"-rc.1", source)

    def test_promotion_requires_correct_pr_parent_and_unchanged_source_tree(self):
        source = self.github.candidate()
        for condition in ("wrong_parent", "changed_tree", "wrong_direction", "wrong_workflow_branch"):
            with self.subTest(condition=condition):
                github = copy.deepcopy(self.github)
                if condition == "wrong_parent": github.promotion_head = "f"*40
                if condition == "changed_tree": github.changed_tree = True
                if condition == "wrong_direction": github.pull_direction = ("feat/bypass", "rc")
                if condition == "wrong_workflow_branch": github.run_branch = "main"
                with self.assertRaises(release.ReleaseError):
                    release.make_plan(github, POLICY, "rc", self.base+"-rc.1", source, now=NOW)

    def test_branch_movement_or_wrong_dispatch_branch_stops_tag_creation(self):
        plan = self.plan()
        with patch.dict(os.environ, {"GITHUB_REF": "refs/heads/main"}):
            with self.assertRaises(release.ReleaseError): release.publish(self.github, POLICY, plan, SOURCE)
        self.github.heads["preview"] = "f"*40
        with patch.dict(os.environ, {"GITHUB_REF": "refs/heads/preview"}):
            with self.assertRaises(release.ReleaseError): release.publish(self.github, POLICY, plan, SOURCE)
        self.assertEqual(self.github.writes, [])

    def test_rc_waits_three_days_and_records_explicit_emergency(self):
        source = self.github.candidate(age=71)
        with self.assertRaises(release.ReleaseError):
            self.plan("rc", self.base+"-rc.1", source)
        with self.assertRaises(release.ReleaseError):
            self.plan("rc", self.base+"-rc.1", source, "short")
        reason = "서비스 장애 대응을 위한 긴급 수정으로 관찰 기간을 단축합니다"
        self.assertEqual(self.plan("rc", self.base+"-rc.1", source, reason)["observation_override_reason"], reason)

    def test_failed_wrong_or_incomplete_source_evidence_cannot_promote(self):
        source = self.github.candidate()
        for condition in ["failed", "wrong_workflow", "wrong_sha", "missing_validation", "draft"]:
            with self.subTest(condition=condition):
                github = copy.deepcopy(self.github)
                if condition == "failed": github.run_conclusion = "failure"
                if condition == "wrong_workflow": github.run_path = ".github/workflows/ci.yml"
                if condition == "wrong_sha": github.tags[source] = "b"*40
                if condition == "missing_validation": github.manifests[source].pop("validation")
                if condition == "draft": github.release_data[source]["draft"] = True
                with self.assertRaises(release.ReleaseError):
                    release.make_plan(github, POLICY, "rc", self.base+"-rc.1", source, now=NOW)

    def test_stable_requires_rc_and_one_day(self):
        source = self.github.candidate("rc", age=23)
        with self.assertRaises(release.ReleaseError):
            self.plan("stable", self.base, source)
        self.github.release_data[source]["published_at"] = (NOW-dt.timedelta(hours=25)).isoformat()
        self.assertEqual(self.plan("stable", self.base, source)["source_sha"], MAIN_SOURCE)

    def test_stable_cannot_skip_rc_or_downgrade(self):
        source = self.github.candidate("preview")
        with self.assertRaises(release.ReleaseError):
            self.plan("stable", self.base, source)
        source = self.github.candidate("rc", age=25)
        self.github.release_data["v9.0.0"] = {"tag_name": "v9.0.0", "draft": False, "prerelease": False,
                                             "published_at": (NOW-dt.timedelta(days=10)).isoformat()}
        with self.assertRaises(release.ReleaseError):
            self.plan("stable", self.base, source)

    def test_stable_has_weekly_limit_and_mandatory_approval(self):
        source = self.github.candidate("rc", age=25)
        self.github.release_data["v0.6.5"] = {"tag_name": "v0.6.5", "draft": False, "prerelease": False,
                                             "published_at": (NOW-dt.timedelta(days=2)).isoformat()}
        with self.assertRaises(release.ReleaseError): self.plan("stable", self.base, source)
        self.github.release_data["v0.6.5"]["published_at"] = (NOW-dt.timedelta(days=8)).isoformat()
        self.plan("stable", self.base, source)
        for reviewers, bypass in [([], False), ([POLICY["reviewer_login"]], True)]:
            self.github.reviewers, self.github.bypass = reviewers, bypass
            with self.assertRaises(release.ReleaseError): self.plan("stable", self.base, source)

    def test_tag_write_requires_verified_sha_and_unchanged_policy(self):
        plan = self.plan()
        with patch.dict(os.environ, {"GITHUB_REF": "refs/heads/preview"}), patch.object(release.dt, "datetime", wraps=dt.datetime) as clock:
            clock.now.return_value = NOW
            with self.assertRaises(release.ReleaseError): release.publish(self.github, POLICY, plan, "b"*40)
            self.assertEqual(self.github.writes, [])
            self.github.policy["preview_hours"] += 1
            with self.assertRaises(release.ReleaseError): release.publish(self.github, POLICY, plan, SOURCE)
            self.assertEqual(self.github.writes, [])
            self.github.policy = copy.deepcopy(POLICY)
            release.publish(self.github, POLICY, plan, SOURCE)
            self.assertEqual(self.github.writes, [("git/refs", {"ref": "refs/tags/"+plan["version"], "sha": SOURCE})])

    def test_public_installation_failure_never_creates_release(self):
        plan = self.plan()
        self.github.tags[plan["version"]] = SOURCE
        report = {"result": "failed", "version": plan["version"], "source_sha": SOURCE,
                  "components": plan["components"]}
        with patch.object(release, "run") as command:
            with self.assertRaises(release.ReleaseError): release.finish(self.github, plan, report)
            command.assert_not_called()

    def test_publication_marks_preview_and_rc_but_not_stable(self):
        for channel in ("preview", "rc", "stable"):
            with self.subTest(channel=channel):
                version = self.base if channel == "stable" else self.base+"-"+channel+".1"
                plan = {"version": version, "channel": channel, "source_sha": SOURCE,
                        "source_tag": "", "branch": POLICY["branches"][channel], "components": release.components(POLICY), "observation_override_reason": ""}
                report = {"result": "passed", "version": version, "source_sha": SOURCE, "components": plan["components"]}
                self.github.tags[version] = SOURCE
                with patch.object(release, "run") as command:
                    release.finish(self.github, plan, report)
                    arguments = command.call_args.args
                    self.assertIn("--verify-tag", arguments)
                    self.assertEqual("--prerelease" in arguments, channel != "stable")
                    self.assertEqual("--latest" in arguments, channel == "stable")

    def test_unknown_api_failures_are_not_treated_as_missing_tags(self):
        github = release.GitHub("NARUBROWN/spine")
        for status in ("401", "403", "500"):
            result = subprocess.CompletedProcess([], 1, json.dumps({"status": status}), "failure")
            with patch.object(release.subprocess, "run", return_value=result), self.assertRaises(release.ReleaseError):
                github.api("git/ref/tags/v0.6.6", missing=True)
        result = subprocess.CompletedProcess([], 1, json.dumps({"status": "404"}), "missing")
        with patch.object(release.subprocess, "run", return_value=result):
            self.assertIsNone(github.api("git/ref/tags/v0.6.6", missing=True))

    def test_public_checks_cannot_silently_skip_brokers_or_old_examples(self):
        with self.assertRaises(release.ReleaseError):
            verify_public.verify({}, Path("."), Path("."), {}, "unused")
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)
            (path/"go.mod").write_text("module example.com/test\n\ngo 1.25.5\n\nrequire github.com/NARUBROWN/spine v0.6.0\n")
            with self.assertRaisesRegex(release.ReleaseError, "전환 전"):
                verify_public.require_migrated_example(path, dict(os.environ, GOWORK="off"))
            (path/"go.mod").write_text("module example.com/test\n\ngo 1.25.5\n\nrequire github.com/NARUBROWN/spine v0.6.5\n")
            verify_public.require_migrated_example(path, dict(os.environ, GOWORK="off"))

    def test_public_go_tests_reject_skip_and_empty_test_selection(self):
        for events in [[], [{"Action": "skip", "Test": "TestBroker"}], [{"Action": "run", "Test": "TestBroker"}, {"Action": "skip", "Test": "TestBroker"}]]:
            result = subprocess.CompletedProcess([], 0, "\n".join(json.dumps(e) for e in events), "")
            with patch.object(verify_public.subprocess, "run", return_value=result), self.assertRaises(release.ReleaseError):
                verify_public.go_tests(["./..."], Path("."), {})
        result = subprocess.CompletedProcess([], 0, json.dumps({"Action": "run", "Test": "TestBroker"})+"\n"+json.dumps({"Action": "pass", "Test": "TestBroker"}), "")
        with patch.object(verify_public.subprocess, "run", return_value=result):
            verify_public.go_tests(["./..."], Path("."), {})


if __name__ == "__main__":
    unittest.main()
