# Discord tag notification action

The shared action reports tag creation for Preview, RC and stable channels. It does not report completed release verification or deployment, create tags or publish releases.

Use the local `.github/actions/discord-tag` action in core. Consuming ecosystem repositories pin the action by full commit SHA. Call it immediately after successful tag creation; tags made with `GITHUB_TOKEN` do not automatically trigger follow-up workflows.

| Input | Meaning |
| --- | --- |
| `plan` | Required publication-plan JSON containing version, channel, repository and source SHA; paths are relative to the caller's workspace. |
| `webhook_preview` | Webhook for `vX.Y.Z-preview.N`. |
| `webhook_rc` | Webhook for `vX.Y.Z-rc.N`. |
| `webhook_release` | Webhook for `vX.Y.Z`. |

Store URLs in repository secrets `DISCORD_WEBHOOK_PREVIEW`, `DISCORD_WEBHOOK_RC` and `DISCORD_WEBHOOK_RELEASE`. Do not log them or expose them as step outputs. Forum/media destinations require the appropriate `thread_id`.

The Go action rejects unknown tag forms and plan/channel mismatches. `N` must be positive. It uses a 20-second connection timeout and `wait=true`, verifies Discord's message ID, and exits 1 on HTTP failures, malformed responses or missing secrets. Configure `continue-on-error: true` so notification failure does not stop verification after a tag was created.

There is no automatic retry. Check the channel after an uncertain response and do not republish a tag just to retry a notification. Dry-run prints a payload without a webhook but still requires the normal publication context. See [release operations](releasing.md).

[Korean original](../ko/maintenance/discord-tag.md).
