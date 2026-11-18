# Run Spine tests

Product tests live under the root `test/` tree. Use the overlay runner so unit tests execute in their original packages. Running only `go test ./...` at the repository root omits those tests and separate example modules.

```sh
go run ./scripts/test test -race ./... -count=1 -shuffle=on -timeout=8m
go run ./scripts/test vet ./...
go run ./scripts/test -module cmd/demo test -race ./... -count=1
go run ./scripts/test -module cmd/reference test -race ./... -count=1
go run ./scripts/test -module examples/kafka-publish test -race ./... -count=1
```

Select compatible local modules in a development `go.work` when validating unreleased source APIs. Do not commit workspace files. Core requires Go 1.25.5; some validation/example dependencies require Go 1.26 or later. Record the actual toolchain used.

| Location | Scope |
| --- | --- |
| `test/unit` | Core, contracts, examples and package-private tests, mapped by overlay. |
| `test` | Application HTTP, WS and event integration. |
| `test/http-platform` | Actual Echo and net/http behavior. |
| `test/openapi` | Runtime descriptions versus actual request behavior. |
| `test/platform-e2e` | Platform and broker contract combinations. |
| `test/nats-e2e` | NATS/JetStream integration. |
| `scripts`, `.github/actions/discord-tag` | Maintenance-tool tests kept with their source. |

```sh
go -C test test -race ./... -count=1
go -C test/http-platform test -race ./... -count=1
go -C test/openapi test -race ./... -count=1
go run ./scripts/verifydocs
go run ./scripts/verifytools
bash scripts/check-module-boundaries.sh
```

The module-boundary check keeps core contracts standard-library-only. Only bootstrap and WS/event execution and binding packages may import the public handler context types; other internal packages use the internal view contracts.

Core CI requires at least 80% statement coverage across core packages, excluding maintenance scripts. Race checks do not replace resource-lifetime checks. Profile retained contexts when changing ownership or cleanup.

Optional broker tests skip when endpoints are missing. Record skips and distinguish static inspection, local source integration and public-tag installation. See [HTTP platforms](http-platforms.md), [OpenAPI](openapi.md), [broker combinations](platforms.md) and [HTTP contract probes](http-contract.md).
