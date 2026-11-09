"""Verifier regressions for empty test execution and the core overlay runner."""
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch


class VerifierTests(unittest.TestCase):
    def test_each_module_requires_at_least_one_passing_test(self):
        scripts = Path(__file__).resolve().parent
        for name in ("verify-http-v3", "verify-http-platforms"):
            spec = importlib.util.spec_from_file_location(name, scripts / (name + ".py"))
            verifier = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(verifier)
            for passing in (False, True):
                with self.subTest(script=name, passing=passing), tempfile.TemporaryDirectory() as folder:
                    root = Path(folder)
                    (root / "go.mod").write_text("module fixture\n\ngo 1.25.5\n")
                    output = root / "output"
                    argv = [name, "--echo-root", str(root), "--output-dir", str(output)]
                    if name == "verify-http-platforms":
                        argv += ["--nethttp-root", str(root)]
                    commands = []

                    def run(command, **kwargs):
                        commands.append(command)
                        if "-json" in command and passing:
                            for action in ("run", "pass"):
                                kwargs["stdout"].write(json.dumps({"Action": action, "Package": "fixture", "Test": "TestOK"}) + "\n")
                        return subprocess.CompletedProcess(command, 0)

                    with patch.object(sys, "argv", argv), patch.object(verifier.subprocess, "run", side_effect=run):
                        self.assertEqual(verifier.main(), 0 if passing else 1)
                    self.assertEqual(json.loads((output / "summary.json").read_text())["success"], passing)
                    if name == "verify-http-v3":
                        self.assertTrue(any(c[:4] == ["go", "run", "./scripts/test", "test"] for c in commands))
                        self.assertTrue(any(c[:4] == ["go", "run", "./scripts/test", "vet"] for c in commands))


if __name__ == "__main__":
    unittest.main()
