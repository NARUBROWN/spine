# Release operations

Release policy lives in [.github/release-policy.json](../../.github/release-policy.json). Read that file for the selected channel's exact core/platform versions and source SHAs. This guide describes the process; it is not a claim that a tag, CI run or promotion has completed.

## Source validation and public installation

Source CI connects the current core to pinned platform source commits in a temporary workspace. Temporary replacements must not leak into published manifests. Platform source CI uses its pinned core source in the same way.

Public verification uses a separate consumer, new module cache, `GOWORK=off` and no local replacements. Record what Go actually selected. A local source workspace success does not substitute for public installation.

The core does not depend on concrete platforms. Platforms have independent tags. A core release may reuse existing compatible platform tags rather than issuing new platform versions.

## Candidate and complete combination

1. Validate core race/vet, policy, release tooling and a public-SHA consumer before creating a new core candidate tag.
2. Build that public tag and save the core-candidate artifact. The tag alone is not a promotable complete combination.
3. Select the exact published platform tags/commits and verify the chosen core together with HTTP, WS, real Kafka/RabbitMQ/NATS, validation and OpenAPI.
4. Save the release manifest and full evidence. `validation.scope: cohort` identifies this complete combination.
5. Promote only from the verified combination. Create a stable core GitHub Release after complete verification.

Tags can become visible as Go modules immediately and are not created atomically across repositories. Never move or overwrite a failed tag. Use a new candidate number.

## Channel setup and publication

```sh
go run ./scripts/release/branches --bootstrap
```

This prints the branch setup plan. Applying repository configuration is a separate explicit operation using the reviewed command's apply flag. It can create missing Preview/RC branches, change protections and publishing environments. Do not move existing stable tags or main history. Required release-environment approval remains a human action.

After a source PR is merged into Preview and source CI succeeds, use the Publish Preview workflow with its full source SHA and a new version. Complete Core Release Combination then validates the published core and selected platforms. RC and stable promotion use the verified prior-stage manifest, required observation periods and target-branch checks. An override reason must be recorded when an authorized observation-period override is used.

A notification failure after tag creation does not authorize recreating the tag. The shared [Discord action](discord-tag.md) reports tag creation separately from completion and deployment.

## Evidence and examples

Keep source SHA, selected versions, CI results, broker evidence and manifests together. Check the workflow's immutable source and expected branch direction. Do not describe source validation as public installation or a candidate tag as complete promotion.

After a stable release, update example applications through the post-release examples workflow and validate each actual app. This source branch's demos may intentionally use APIs newer than a published preview; source tests do not prove installation of that older tag.

## Local checks

```sh
go run ./scripts/verifytools
go run ./scripts/verifydocs
go test -race ./.github/actions/discord-tag ./scripts/release/... ./scripts/verifiedsource
node --test scripts/release/branch_flow.test.cjs
go run ./scripts/release/workspace validate-policy
go run ./scripts/release/workspace check-module --channel preview
```

Inspect each workflow and CLI help before publication. Version declarations, documentation checks, source evidence and public consumer evidence must agree. Local checks do not publish tags or prove remote CI success.

[Korean original](../ko/maintenance/releasing.md).
