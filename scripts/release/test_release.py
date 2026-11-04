"""발행 전에 막아야 할 잘못된 승격과 공개 검증 누락을 검사합니다."""

import base64
import copy
import datetime as dt
import io
import json
import os
from pathlib import Path
import tempfile
import subprocess
import sys
import unittest
import zipfile
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "scripts/release"))

import release
import verify_public


NOW = dt.datetime(2026, 11, 10, tzinfo=dt.timezone.utc)
SOURCE = "a" * 40
RC_SOURCE = "b" * 40
MAIN_SOURCE = "c" * 40
TREE = "d" * 40
POLICY = json.loads((Path(__file__).resolve().parents[2] / ".github/release-policy.json").read_text())


class FakeGitHub:
    repository = POLICY["module"].removeprefix("github.com/")

    def __init__(self):
        self.tags = {}
        self.release_data = {}
        self.manifests = {}
        self.artifacts = {}
        self.writes = []
        self.comparison = "ahead"
        self.heads = {"preview": SOURCE, "rc": RC_SOURCE, "main": MAIN_SOURCE}
        self.promotion_head = None
        self.changed_tree = False
        self.pull_direction = None
        self.run_conclusion = "success"
        self.run_path = ".github/workflows/preview.yml"
        self.run_attempt = 1
        self.policy = copy.deepcopy(POLICY)
        self.source_policy = None
        self.reviewers = [POLICY["reviewer_login"]]
        self.bypass = False

    def tag_sha(self, tag):
        return self.tags.get(tag)

    def releases(self):
        return list(self.release_data.values())

    def candidate_evidence(self, tag):
        return {"manifest": self.manifests[tag], "artifact": self.artifacts[tag],
                "workflow_run": self.api("actions/runs/1")}

    def api(self, path, *, missing=False, method="GET", data=None):
        if path.startswith("git/trees/"):
            changed = path.split("/")[2].split("?")[0] != TREE
            return {"tree": [{"path": "source.go", "mode": "100644", "type": "blob", "sha": ("2" if changed else "1") * 40}]}
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
        if path.startswith("actions/runs/"):
            return {"id": 1, "run_attempt": self.run_attempt, "conclusion": self.run_conclusion, "status": "completed", "event": "workflow_dispatch", "path": self.run_path, "head_branch": self.run_branch, "head_sha": self.run_sha}
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
        self.artifacts[tag] = {"id": 10, "name": "release-manifest-" + tag, "expired": False,
                               "digest": "sha256:" + "e" * 64,
                               "created_at": (NOW-dt.timedelta(hours=age)).isoformat(),
                               "workflow_run": {"id": 1, "head_sha": sha, "head_branch": POLICY["branches"][channel]}}
        self.manifests[tag] = {"schema_version": "spine.release-manifest/v1", "channel": channel,
                               "version": tag, "source_sha": sha, "branch": POLICY["branches"][channel], "source_tree": TREE, "workflow_run_id": "1",
                               "workflow_run_attempt": "1", "repository": self.repository, "components": release.components(POLICY, channel),
                               "kind": POLICY["kind"], "module": POLICY["module"], "source_fingerprint": release.source_fingerprint(self, sha)}
        self.manifests[tag]["validation"] = {"result": "passed", "version": tag, "source_sha": sha,
                                             "components": release.components(POLICY, channel), "scope": "cohort" if POLICY["kind"] == "core" else "platform"}
        self.manifests[tag]["validation"]["selected_modules"] = release.expected_modules(self.manifests[tag], self.manifests[tag]["validation"]["scope"])
        self.run_path = ".github/workflows/" + ("complete.yml" if POLICY["kind"] == "core" else release.WORKFLOWS[channel])
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
                if condition == "wrong_parent": github.promotion_head = "f"*40; github.comparison = "diverged"
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
        for condition in ["failed", "wrong_workflow", "wrong_sha", "missing_validation", "expired", "wrong_repository",
                          "wrong_artifact_run", "wrong_artifact_sha", "wrong_attempt", "wrong_tree"]:
            with self.subTest(condition=condition):
                github = copy.deepcopy(self.github)
                if condition == "failed": github.run_conclusion = "failure"
                if condition == "wrong_workflow": github.run_path = ".github/workflows/ci.yml"
                if condition == "wrong_sha": github.tags[source] = "b"*40
                if condition == "missing_validation": github.manifests[source].pop("validation")
                if condition == "expired": github.artifacts[source]["expired"] = True
                if condition == "wrong_repository": github.manifests[source]["repository"] = "elsewhere/spine"
                if condition == "wrong_artifact_run": github.artifacts[source]["workflow_run"]["id"] = 2
                if condition == "wrong_artifact_sha": github.artifacts[source]["workflow_run"]["head_sha"] = "f" * 40
                if condition == "wrong_attempt": github.run_attempt = 2
                if condition == "wrong_tree": github.manifests[source]["source_tree"] = "f" * 40
                with self.assertRaises(release.ReleaseError):
                    release.make_plan(github, POLICY, "rc", self.base+"-rc.1", source, now=NOW)

    def test_stable_requires_rc_and_one_day(self):
        source = self.github.candidate("rc", age=23)
        with self.assertRaises(release.ReleaseError):
            self.plan("stable", self.base, source)
        self.github.artifacts[source]["created_at"] = (NOW-dt.timedelta(hours=25)).isoformat()
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
        self.github.release_data["v0.6.5" if POLICY["kind"] == "core" else "v0.1.0"] = {"tag_name": "v0.6.5" if POLICY["kind"] == "core" else "v0.1.0", "draft": False, "prerelease": False,
                                             "published_at": (NOW-dt.timedelta(days=2)).isoformat()}
        with self.assertRaises(release.ReleaseError): self.plan("stable", self.base, source)
        self.github.release_data["v0.6.5" if POLICY["kind"] == "core" else "v0.1.0"]["published_at"] = (NOW-dt.timedelta(days=8)).isoformat()
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

    def test_all_channels_record_manifest_and_only_stable_creates_github_release(self):
        for channel in ("preview", "rc", "stable"):
            with self.subTest(channel=channel):
                version = self.base if channel == "stable" else self.base+"-"+channel+".1"
                plan = {"version": version, "channel": channel, "source_sha": SOURCE,
                        "source_tag": "", "branch": POLICY["branches"][channel], "components": release.components(POLICY, channel), "observation_override_reason": "", "kind": POLICY["kind"], "module": POLICY["module"]}
                report = {"result": "passed", "version": version, "source_sha": SOURCE, "components": plan["components"], "scope": "cohort" if POLICY["kind"] == "core" else "platform"}
                report["selected_modules"] = release.expected_modules(plan, report["scope"])
                self.github.tags[version] = SOURCE
                with tempfile.TemporaryDirectory() as directory, patch.object(release, "run") as command, patch.object(release, "complete_plan", return_value=plan):
                    manifest_path = Path(directory)/"release-manifest.json"
                    release.finish(self.github, plan, report, manifest_path)
                    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
                    self.assertEqual(manifest["version"], version)
                    self.assertEqual(manifest["validation"], report)
                    if channel == "stable":
                        command.assert_called_once()
                        arguments = command.call_args.args
                        self.assertEqual(arguments[:3], ("gh", "release", "create"))
                        self.assertIn("--verify-tag", arguments)
                        self.assertIn("--latest", arguments)
                        self.assertNotIn("--prerelease", arguments)
                        self.assertIn(str(manifest_path.resolve()), arguments)
                    else:
                        command.assert_not_called()

    def test_exported_candidate_manifest_promotes_without_a_github_release(self):
        with patch.dict(os.environ, {"GITHUB_RUN_ID": "1", "GITHUB_RUN_ATTEMPT": "1"}):
            for channel, next_channel, source_sha, version in [
                ("preview", "rc", SOURCE, self.base+"-preview.1"),
                ("rc", "stable", RC_SOURCE, self.base+"-rc.1"),
            ]:
                with self.subTest(channel=channel), tempfile.TemporaryDirectory() as directory:
                    previous = SOURCE if channel == "preview" else self.github.candidate("preview")
                    plan = self.plan(channel, version, previous)
                    self.github.candidate(channel, age=73)
                    manifest_path = Path(directory)/"release-manifest.json"
                    report = {"result": "passed", "version": version, "source_sha": source_sha, "components": plan["components"], "scope": "cohort" if POLICY["kind"] == "core" else "platform"}
                    report["selected_modules"] = release.expected_modules(plan, report["scope"])
                    with patch.object(release, "run") as command, patch.object(release, "complete_plan", return_value=plan):
                        release.finish(self.github, plan, report, manifest_path)
                        command.assert_not_called()
                    self.github.manifests[version] = json.loads(manifest_path.read_text(encoding="utf-8"))
                    next_version = self.base if next_channel == "stable" else self.base+"-rc.1"
                    promoted = self.plan(next_channel, next_version, version)
                    self.assertEqual(promoted["upstream_source_sha"], source_sha)
                    self.assertEqual(self.github.release_data, {})

    def test_candidate_observation_uses_artifact_time_and_rejects_future_evidence(self):
        source = self.github.candidate(age=71)
        self.github.release_data[source] = {"published_at": (NOW-dt.timedelta(days=10)).isoformat()}
        with self.assertRaisesRegex(release.ReleaseError, "관찰 기간 부족"):
            self.plan("rc", self.base+"-rc.1", source)
        self.github.artifacts[source]["created_at"] = (NOW+dt.timedelta(hours=1)).isoformat()
        with self.assertRaisesRegex(release.ReleaseError, "현재보다 뒤"):
            self.plan("rc", self.base+"-rc.1", source, "서비스 장애 대응을 위한 긴급 수정으로 관찰 기간을 단축합니다")

    def test_replacing_source_artifact_while_approval_is_pending_stops_tag_write(self):
        source = self.github.candidate()
        plan = self.plan("rc", self.base+"-rc.1", source)
        self.github.artifacts[source]["id"] = 11
        with patch.dict(os.environ, {"GITHUB_REF": "refs/heads/rc"}), patch.object(release.dt, "datetime", wraps=dt.datetime) as clock:
            clock.now.return_value = NOW
            with self.assertRaisesRegex(release.ReleaseError, "승인한 계획"):
                release.publish(self.github, POLICY, plan, RC_SOURCE)
        self.assertEqual(self.github.writes, [])

    def test_repository_settings_endpoint_has_no_trailing_slash(self):
        result = subprocess.CompletedProcess([], 0, '{"default_branch":"preview"}', "")
        with patch.object(release.subprocess, "run", return_value=result) as command:
            actual = release.GitHub("NARUBROWN/spine").api("", method="PATCH", data={"default_branch": "preview"})
            self.assertEqual(actual["default_branch"], "preview")
            self.assertEqual(command.call_args.args[0][4], "repos/NARUBROWN/spine")

    def test_unknown_api_failures_are_not_treated_as_missing_tags(self):
        github = release.GitHub("NARUBROWN/spine")
        for status in ("401", "403", "500"):
            result = subprocess.CompletedProcess([], 1, json.dumps({"status": status}), "failure")
            with patch.object(release.subprocess, "run", return_value=result), self.assertRaises(release.ReleaseError):
                github.api("git/ref/tags/v0.6.6", missing=True)
        result = subprocess.CompletedProcess([], 1, json.dumps({"status": "404"}), "missing")
        with patch.object(release.subprocess, "run", return_value=result):
            self.assertIsNone(github.api("git/ref/tags/v0.6.6", missing=True))

    def test_public_checks_cannot_silently_skip_brokers(self):
        with self.assertRaises(release.ReleaseError):
            verify_public.verify({}, Path("."), Path("."), {}, "unused")

    def test_public_go_tests_reject_skip_and_empty_test_selection(self):
        for events in [[], [{"Action": "skip", "Test": "TestBroker"}], [{"Action": "run", "Test": "TestBroker"}, {"Action": "skip", "Test": "TestBroker"}]]:
            result = subprocess.CompletedProcess([], 0, "\n".join(json.dumps(e) for e in events), "")
            with patch.object(verify_public.subprocess, "run", return_value=result), self.assertRaises(release.ReleaseError):
                verify_public.go_tests(["./..."], Path("."), {})
        result = subprocess.CompletedProcess([], 0, json.dumps({"Action": "run", "Test": "TestBroker"})+"\n"+json.dumps({"Action": "pass", "Test": "TestBroker"}), "")
        with patch.object(verify_public.subprocess, "run", return_value=result):
            verify_public.go_tests(["./..."], Path("."), {})


class CandidateArtifactTests(unittest.TestCase):
    def setUp(self):
        self.github = release.GitHub("NARUBROWN/spine")
        self.tag = POLICY["next_version"] + "-preview.1"
        self.payload = {"version": self.tag, "source_sha": SOURCE}
        self.archive = self.package("release-manifest.json", json.dumps(self.payload))
        self.artifact = {"id": 42, "name": "release-manifest-"+self.tag, "expired": False,
                         "digest": "sha256:"+release.hashlib.sha256(self.archive).hexdigest(),
                         "workflow_run": {"id": 7}}
        self.items = [self.artifact]
        self.paths = []

    def package(self, name, content):
        data = io.BytesIO()
        with zipfile.ZipFile(data, "w") as archive:
            archive.writestr(name, content)
        return data.getvalue()

    def api(self, path):
        self.paths.append(path)
        if path.startswith("actions/artifacts?"):
            return {"artifacts": self.items}
        if path == "actions/runs/7":
            return {"id": 7, "conclusion": "success"}
        raise AssertionError(path)

    def download(self, *args, **kwargs):
        self.assertEqual(args[0], ["gh", "api", "repos/NARUBROWN/spine/actions/artifacts/42/zip"])
        kwargs["stdout"].write(self.archive)
        return subprocess.CompletedProcess(args[0], 0, stderr=b"")

    def test_downloads_exact_artifact_id_and_checks_archive_digest(self):
        with patch.object(self.github, "api", side_effect=self.api), patch.object(release.subprocess, "run", side_effect=self.download):
            evidence = self.github.candidate_evidence(self.tag)
        self.assertEqual(evidence["manifest"], self.payload)
        self.assertEqual(evidence["workflow_run"]["id"], 7)
        self.assertTrue(all("releases" not in path for path in self.paths))

    def test_missing_duplicate_or_expired_artifacts_cannot_promote(self):
        for items in ([], [self.artifact, self.artifact], [dict(self.artifact, expired=True)]):
            with self.subTest(items=items):
                self.items = items
                with patch.object(self.github, "api", side_effect=self.api), patch.object(release.subprocess, "run") as command:
                    with self.assertRaises(release.ReleaseError):
                        self.github.candidate_evidence(self.tag)
                    command.assert_not_called()

    def test_corrupt_or_replaced_archive_and_missing_manifest_are_rejected(self):
        original = self.archive
        for condition in ("replaced", "corrupt", "missing_manifest"):
            with self.subTest(condition=condition):
                self.archive = b"corrupt" if condition != "missing_manifest" else self.package("wrong.json", "{}")
                self.artifact["digest"] = "sha256:" + release.hashlib.sha256(original if condition == "replaced" else self.archive).hexdigest()
                with patch.object(self.github, "api", side_effect=self.api), patch.object(release.subprocess, "run", side_effect=self.download):
                    with self.assertRaises(release.ReleaseError):
                        self.github.candidate_evidence(self.tag)

    def test_download_failure_is_not_treated_as_candidate_evidence(self):
        failed = subprocess.CompletedProcess([], 1, stderr=b"artifact unavailable")
        with patch.object(self.github, "api", side_effect=self.api), patch.object(release.subprocess, "run", return_value=failed):
            with self.assertRaisesRegex(release.ReleaseError, "다운로드"):
                self.github.candidate_evidence(self.tag)

    def test_artifact_search_reads_later_pages(self):
        def api(path):
            if path.endswith("&page=1"):
                return {"artifacts": [dict(self.artifact, name="unrelated") for _ in range(100)]}
            if path.endswith("&page=2"):
                return {"artifacts": [self.artifact]}
            return self.api(path)
        with patch.object(self.github, "api", side_effect=api), patch.object(release.subprocess, "run", side_effect=self.download):
            self.assertEqual(self.github.candidate_evidence(self.tag)["manifest"], self.payload)


if __name__ == "__main__":
    unittest.main()
