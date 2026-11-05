"""태그 분기와 실제 HTTP 전송 경계에서 잘못된 Discord 알림을 검사합니다."""

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))

import notify_discord as notify
from release import ReleaseError


PLAN = {"version": "v0.7.0-preview.2", "channel": "preview", "source_sha": "a" * 40,
        "repository": "NARUBROWN/spine"}
RUN_URL = "https://github.com/NARUBROWN/spine/actions/runs/123"


class DiscordNotificationTests(unittest.TestCase):
    def test_tag_routes_and_valid_json(self):
        for suffix, channel, secret in [
            ("-preview.2", "preview", "DISCORD_WEBHOOK_PREVIEW"),
            ("-rc.1", "rc", "DISCORD_WEBHOOK_RC"),
            ("", "stable", "DISCORD_WEBHOOK_RELEASE"),
        ]:
            with self.subTest(channel=channel):
                plan = dict(PLAN, version="v0.7.0" + suffix, channel=channel)
                selected, payload = notify.build_payload(plan, '한글 "이름"\n$(`test`)', RUN_URL)
                self.assertEqual(selected, channel)
                self.assertEqual(notify.ROUTES[selected][0], secret)
                decoded = json.loads(json.dumps(payload, ensure_ascii=False))
                embed = decoded["embeds"][0]
                self.assertEqual(embed["url"], f"https://github.com/NARUBROWN/spine/tree/{plan['version']}")
                self.assertIn("아직 완료되지 않았습니다", embed["description"])
                self.assertEqual(decoded["allowed_mentions"], {"parse": []})

    def test_invalid_tag_or_inconsistent_plan_is_rejected(self):
        for changes in [{"version": "v0.7.0-custom.1"}, {"channel": "stable"}, {"source_sha": "invalid"}]:
            with self.subTest(changes=changes), self.assertRaises(ReleaseError):
                notify.build_payload(dict(PLAN, **changes), "NARUBROWN", RUN_URL)

    def test_unicode_truncation_preserves_valid_text_and_limit(self):
        for value in ['한글 "본문"\n' * 600, "🚀" * 300]:
            shortened = notify.shorten(value, 256)
            self.assertLessEqual(len(shortened.encode("utf-16-le")) // 2, 256)
            self.assertTrue(shortened.endswith("..."))
            self.assertNotIn("\ufffd", shortened)

    def test_wait_confirmation_preserves_thread(self):
        url = notify.webhook_url("https://discord.com/api/webhooks/123/token?thread_id=456&wait=false")
        self.assertEqual(url, "https://discord.com/api/webhooks/123/token?thread_id=456&wait=true")
        with self.assertRaises(ReleaseError):
            notify.webhook_url("http://discord.com/api/webhooks/123/token")
        with self.assertRaises(ReleaseError):
            notify.webhook_url("https://example.com/api/webhooks/123/token")

    def test_http_delivery_and_failures(self):
        requests = []
        responses = [(200, {"id": "789"}), (400, {"message": "invalid"}),
                     (429, {"retry_after": 1}), (500, {}), (200, {})]

        class Handler(BaseHTTPRequestHandler):
            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                requests.append((self.path, body))
                status, response = responses.pop(0)
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(json.dumps(response).encode())

            def log_message(self, *args):
                pass

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        try:
            _, payload = notify.build_payload(PLAN, "NARUBROWN", RUN_URL)
            url = f"http://127.0.0.1:{server.server_port}/api/webhooks/123/test-token?wait=true"
            notify.send(url, payload)
            for status in (400, 429, 500):
                with self.subTest(status=status), self.assertRaisesRegex(ReleaseError, f"HTTP {status}"):
                    notify.send(url, payload)
            with self.assertRaisesRegex(ReleaseError, "메시지"):
                notify.send(url, payload)
            self.assertEqual(len(requests), 5)
            self.assertTrue(all(path.endswith("wait=true") and data == payload for path, data in requests))
        finally:
            server.shutdown()
            server.server_close()
            worker.join()

    def test_dry_run_needs_no_secret_and_missing_secret_is_reported(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "plan.json"
            path.write_text(json.dumps(PLAN))
            env = {key: value for key, value in os.environ.items() if not key.startswith("DISCORD_WEBHOOK_")}
            env.update(GITHUB_REPOSITORY=PLAN["repository"], GITHUB_ACTOR="NARUBROWN", GITHUB_RUN_ID="123")
            command = [sys.executable, str(Path(notify.__file__)), "--plan", str(path)]
            result = subprocess.run(command + ["--dry-run"], env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(result.stdout)["embeds"][0]["color"], 15105570)
            result = subprocess.run(command, env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 1)
            self.assertIn("DISCORD_WEBHOOK_PREVIEW", result.stderr)


if __name__ == "__main__":
    unittest.main()
