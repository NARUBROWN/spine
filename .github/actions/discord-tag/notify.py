#!/usr/bin/env python3
"""태그 생성 직후 버전 형식에 맞는 Discord 채널에 알림을 보냅니다."""

import argparse
import datetime as dt
import json
import os
from pathlib import Path
import re
import sys
import urllib.error
import urllib.parse
import urllib.request

ROUTES = {
    "preview": ("DISCORD_WEBHOOK_PREVIEW", 15105570, "🧪 Preview Tag"),
    "rc": ("DISCORD_WEBHOOK_RC", 3447003, "📦 Release Candidate Tag"),
    "stable": ("DISCORD_WEBHOOK_RELEASE", 5763719, "🚀 Stable Tag"),
}
TAG = re.compile(r"v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-(preview|rc)\.([1-9]\d*))?\Z")
SHA = re.compile(r"[0-9a-f]{40}\Z")


class NotificationError(Exception):
    pass


def tag_channel(tag):
    match = TAG.fullmatch(tag)
    if not match:
        raise NotificationError("태그는 vX.Y.Z-preview.N, vX.Y.Z-rc.N 또는 vX.Y.Z 형식이어야 합니다")
    return match[4] or "stable"


def shorten(value, limit):
    # Discord가 UTF-16 단위로 세는 경우에도 이모지가 한도를 넘지 않게 합니다.
    if len(value.encode("utf-16-le")) // 2 <= limit:
        return value
    prefix = value.encode("utf-16-le")[:(limit - 3) * 2].decode("utf-16-le", errors="ignore")
    return prefix + "..."


def build_payload(plan, actor, run_url):
    tag = plan["version"]
    channel = tag_channel(tag)
    if channel != plan.get("channel", channel):
        raise NotificationError("태그와 발행 계획의 채널이 일치하지 않습니다")
    if not SHA.fullmatch(plan["source_sha"]):
        raise NotificationError("태그의 소스 SHA가 올바르지 않습니다")
    repository = plan["repository"]
    tag_url = f"https://github.com/{repository}/tree/{urllib.parse.quote(tag, safe='')}"
    _, color, title = ROUTES[channel]
    project = repository.split("/")[-1]
    return channel, {
        "username": "Spine Bot",
        "avatar_url": "https://raw.githubusercontent.com/NARUBROWN/spine/main/assets/spine-logo.png",
        "allowed_mentions": {"parse": []},
        "embeds": [{
            "title": shorten(f"{title} - {project} {tag}", 256),
            "url": tag_url,
            "color": color,
            "description": "새 태그가 생성되었습니다. 릴리스 검증과 배포 상태는 프로젝트의 발행 기록에서 확인하세요.",
            "fields": [
                {"name": "🏷️ Tag", "value": f"`{shorten(tag, 1022)}`", "inline": True},
                {"name": "👤 실행자", "value": f"[{actor}](https://github.com/{actor})", "inline": True},
                {"name": "실행 기록", "value": f"[GitHub Actions에서 확인]({run_url})"},
            ],
            "footer": {"text": "Spine Web Framework | GitHub Actions"},
            "timestamp": dt.datetime.now(dt.timezone.utc).isoformat(timespec="seconds"),
        }],
    }


def webhook_url(value):
    parts = urllib.parse.urlsplit(value.strip())
    if (parts.scheme != "https" or parts.hostname not in {"discord.com", "discordapp.com"}
            or parts.username or parts.password or parts.port not in (None, 443)
            or not parts.path.startswith("/api/webhooks/")):
        raise NotificationError("Discord webhook URL 형식을 확인하세요")
    query = dict(urllib.parse.parse_qsl(parts.query))
    query["wait"] = "true"
    return urllib.parse.urlunsplit((parts.scheme, parts.netloc, parts.path,
                                   urllib.parse.urlencode(query), ""))


def send(url, payload):
    request = urllib.request.Request(url, data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
                                     headers={"Content-Type": "application/json", "User-Agent": "Spine-Release-Notifier"},
                                     method="POST")
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            message = json.load(response)
        if not isinstance(message, dict) or not message.get("id"):
            raise NotificationError("Discord에서 메시지 생성 확인을 받지 못했습니다")
    except urllib.error.HTTPError as exc:
        status = exc.code
        exc.close()
        raise NotificationError(f"Discord 전송 실패: HTTP {status}") from None
    except (OSError, ValueError):
        # 예외 원문에는 webhook URL과 토큰이 들어갈 수 있으므로 출력하지 않습니다.
        raise NotificationError("Discord 전송 실패: 연결 또는 응답 형식을 확인하세요") from None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plan", default=os.environ.get("NOTIFICATION_PLAN") or "release-plan.json")
    parser.add_argument("--dry-run", action="store_true", help="전송 없이 Discord payload만 출력")
    args = parser.parse_args()
    plan = json.loads(Path(args.plan).read_text(encoding="utf-8"))
    if plan["repository"] != os.environ["GITHUB_REPOSITORY"]:
        raise NotificationError("발행 계획과 실행 저장소가 일치하지 않습니다")
    run_url = f"https://github.com/{plan['repository']}/actions/runs/{os.environ['GITHUB_RUN_ID']}"
    channel, payload = build_payload(plan, os.environ["GITHUB_ACTOR"], run_url)
    if args.dry_run:
        print(json.dumps(payload, ensure_ascii=False, indent=2))
        return
    secret = ROUTES[channel][0]
    value = os.environ.get(secret, "")
    if not value.strip():
        raise NotificationError(f"{secret} secret이 등록되지 않았습니다")
    send(webhook_url(value), payload)
    print(f"Discord {channel} 채널에 태그 알림을 보냈습니다")


if __name__ == "__main__":
    try:
        main()
    except NotificationError as exc:
        print(f"::warning::{exc}", file=sys.stderr)
        sys.exit(1)
    except (KeyError, OSError, ValueError):
        # 알림 실패는 발행을 되돌리지 않으며 workflow 단계에 경고를 남깁니다.
        print("::warning::Discord 태그 알림을 보내지 못했습니다. 채널별 secret과 webhook 응답을 확인하세요.", file=sys.stderr)
        sys.exit(1)
