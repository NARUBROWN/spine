import base64
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location("updater", Path(__file__).resolve().parents[1] / "update-openapi-ui.py")
updater = importlib.util.module_from_spec(spec)
spec.loader.exec_module(updater)


class UpdaterTests(unittest.TestCase):
    def test_license_expressions_require_a_complete_permitted_choice(self):
        for expression in ["MIT", "(MPL-2.0 OR Apache-2.0)", "(MIT OR GPL-3.0) AND BSD-3-Clause"]:
            self.assertTrue(updater.permitted(expression), expression)
        for expression in ["GPL-3.0", "MIT AND GPL-3.0", "(MIT OR BSD-3-Clause) AND GPL-3.0", "MIT WITH unknown-exception", "MIT OR", "(MIT"]:
            self.assertFalse(updater.permitted(expression), expression)

    def test_tarball_integrity_rejects_changed_bytes(self):
        entry = {"resolved": updater.REGISTRY + "fixture.tgz", "integrity": "sha512-" + base64.b64encode(hashlib.sha512(b"expected").digest()).decode()}
        with patch.object(updater, "download", return_value=b"changed"):
            with self.assertRaisesRegex(ValueError, "integrity"):
                updater.archive(entry)

    def test_typings_archives_do_not_require_package_directory_name(self):
        buffer = io.BytesIO()
        with tarfile.open(fileobj=buffer, mode="w:gz") as archive:
            payload = b"MIT license terms"
            member = tarfile.TarInfo("hast/LICENSE")
            member.size = len(payload)
            archive.addfile(member, io.BytesIO(payload))
        data = buffer.getvalue()
        entry = {"resolved": updater.REGISTRY + "fixture.tgz", "integrity": "sha512-" + base64.b64encode(hashlib.sha512(data).digest()).decode()}
        with patch.object(updater, "download", return_value=data):
            self.assertEqual(updater.archive(entry)["LICENSE"], payload)

    def test_file_hash_drift_fails_offline_verification(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            tools = root / "scripts/openapi-ui"
            tools.mkdir(parents=True)
            (tools / "package.json").write_text(json.dumps({"dependencies": {"redoc": "2.5.4"}}))
            (root / "asset.js").write_text("changed")
            manifest = tools / "manifest.json"
            manifest.write_text(json.dumps({"distributions": {"redoc": {"version": "2.5.4"}}, "files": {"asset.js": hashlib.sha256(b"expected").hexdigest()}}))
            with patch.object(updater, "ROOT", root), patch.object(updater, "TOOLS", tools), patch.object(updater, "MANIFEST", manifest):
                with self.assertRaisesRegex(ValueError, "changed"):
                    updater.check()


if __name__ == "__main__":
    unittest.main()
