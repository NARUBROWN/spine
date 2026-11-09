"""예제 검증은 후보 발행과 분리하고 정식 고정 버전을 실제로 갱신해야 합니다."""

import copy
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import post_examples
from test_channels import core_policy
from test_release import FakeGitHub, SOURCE
from release import ReleaseError


class PostReleaseExamplesTests(unittest.TestCase):
    def test_preview_rc_and_non_main_dispatch_cannot_run_post_release_examples(self):
        for branch, version in (("main", "v0.7.0-preview.1"), ("main", "v0.7.0-rc.1"), ("preview", "v0.7.0")):
            with self.subTest(branch=branch, version=version), patch.dict(os.environ, {"GITHUB_REF": "refs/heads/"+branch}):
                with self.assertRaises(ReleaseError):
                    post_examples.verify_examples({}, version, Path("."), {}, None)

    def test_old_example_pins_are_not_rewritten_to_make_verification_pass(self):
        policy = core_policy()
        github = FakeGitHub()
        github.policy = policy
        github.tags["v0.7.0"] = SOURCE
        policy["post_release_examples"] = {
            "chat": {"repository": "NARUBROWN/spine-simple-chat-demo", "ref": SOURCE},
            "msa": {"repository": "NARUBROWN/spine-simple-msa-demo", "ref": SOURCE}}
        manifest = {"components": policy["channels"]["stable"]}
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            chat = root / "chat"
            chat.mkdir()
            original = "module example.com/chat\n\ngo 1.26.0\n\nrequire github.com/NARUBROWN/spine v0.6.0\n"
            (chat / "go.mod").write_text(original)
            with patch.dict(os.environ, {"GITHUB_REF": "refs/heads/main"}), \
                    patch.object(post_examples, "verify_source_candidate", return_value=({}, manifest)), \
                    patch.object(post_examples, "checkout_example", return_value=chat), \
                    patch.object(post_examples, "execute") as execute:
                with self.assertRaisesRegex(ReleaseError, "updating the example go.mod to the stable"):
                    post_examples.verify_examples(policy, "v0.7.0", root,
                        dict(os.environ, GOWORK="off", SPINE_TEST_KAFKA_BROKER="localhost:19092", SPINE_TEST_RABBITMQ_URL="amqp://localhost"), github)
                self.assertEqual((chat / "go.mod").read_text(), original)
                execute.assert_not_called()


if __name__ == "__main__":
    unittest.main()
