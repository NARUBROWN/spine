<p align="center">
  <a href="README.md">한국어</a> | English | <a href="README.ja.md">日本語</a> | <a href="README.zh-Hans.md">简体中文</a> | <a href="README.zh-Hant.md">繁體中文</a>
</p>

> **Release channels: Preview → RC → Stable.** This source targets `v0.7.0`. See the [release process](RELEASING.md), [import migration guide](docs/migration/v0.7.0.md), and [published releases](https://github.com/NARUBROWN/spine/releases).

> **v0.6.5** unifies public HTTP, WebSocket, event and validator contracts with independent implementation selection. The core uses only the standard library. See the [v0.6.5 migration guide](docs/migration/v0.6.5.md).

> From Spine `v0.6.0`, start new projects with [Spine CLI `new`](https://github.com/NARUBROWN/spine-cli#빠른-시작). The CLI configures the Echo platform and dependencies. For existing apps, see the [migration guide](docs/migration/http-adapter-modules.md).
>
> The development source uses `spine.WithEventAdapter` with Kafka/RabbitMQ platforms. Replace the old broker boot options using the [event platform migration guide](docs/migration/event-platforms.en.md).

> The development source separates WebSocket through `contract/websocket` and the [Gorilla platform](https://github.com/NARUBROWN/spine-gorilla-platform). Select both implementations with `spine.New(echoPlatform.Option(), gorillaPlatform.Option())`. See the [WebSocket migration guide](docs/migration/websocket-platforms.en.md).

# 🦴 Spine
<p align="center">
  <img src="assets/spine-logo.png" alt="Spine" width="420" />
</p>

**Spine is a backend framework that makes request execution explicit.**  
It does not hide how a request is resolved and executed.

## Core ideas

- Fixed execution order and responsibilities
- Method signature is the API contract
- Semantic input types: `path.*`, `query.*`, `httperr.*`

## Free enterprise adoption support

[![Spine enterprise adoption support: free guidance, 24/7 inquiries, and maintainer replies as available](assets/enterprise-adoption/banner.png)](https://github.com/NARUBROWN/spine/issues/new?title=%5BAdoption%20support%5D)

Bring the business architecture and development practices you know from Spring or NestJS to Go.
Any company or development team evaluating or using Spine can request free support directly from the maintainer.
There is no limit on the number of participating companies and no fixed support end date.

- Assess whether Spine fits your service and review the responsibilities and dependencies of controllers, services, and repositories.
- Review initial setup and relevant code to help with dependency injection (DI) and framework usage.
- Reproduce errors and assess workarounds or fixes based on the cause.

Submit inquiries 24/7. The maintainer responds when available, with no guaranteed response or recovery time.
Support takes place asynchronously through text, code, and logs. Meetings can be arranged when further discussion is needed.

Inquiries are prioritized by production impact and available workarounds.
Within the same priority, arrival order and the impact on other users are also considered.

| Inquiry type | Criteria |
|---|---|
| Urgent | A core production function is unavailable or a data integrity incident is in progress |
| Adoption blocker | A build, startup, or core function error prevents adoption or development |
| General | Usage or architecture questions, focused code reviews, or errors with a workaround |
| Improvement | Suggestions for features, usability, documentation, or examples |

Your team develops business features, operates the service, and pays for infrastructure and external services.
Full project development, continuous operations, or ongoing development of features specific to your company require a separate scope discussion.

**[Start an adoption or support inquiry](https://github.com/NARUBROWN/spine/issues/new?title=%5BAdoption%20support%5D)**

Describe your service, current stack, intended adoption scope, and questions.
For errors, include your Spine and Go versions, expected and actual behavior, and minimal reproduction steps.
GitHub Issues are public. Share a sanitized summary or minimal reproduction without confidential information.

## Recommended environment

- Recommended Go version: **1.27.3**

## Links

- Official site: https://spine.na2ru2.me/en/
- Spine CLI (project generation and execution-contract inspection): https://github.com/NARUBROWN/spine-cli
- Bun ORM + Swagger integration example project: https://github.com/NARUBROWN/spine-user-demo
- Kafka MSA example project: https://github.com/NARUBROWN/spine-simple-msa-demo
- Simple WebSocket chat example project: https://github.com/NARUBROWN/spine-simple-chat-demo

---

## Get started with Spine CLI

From Spine `v0.6.0`, `spine new` is the official way to create a project. Follow the [Spine CLI installation guide](https://github.com/NARUBROWN/spine-cli#권장-환경과-설치) to install the `spine` command and check its supported versions.

`new` selects compatible Spine, Echo platform, and Echo versions using the verified recipe bundled with the installed CLI. Check the generated `go.mod` for the selected versions.

Create a project and start its development server.

The example below installs the stable CLI with `@latest`; its recipe may target a different core version from this Preview checkout. For HTTP v3 Preview, replace the install command with `go install github.com/NARUBROWN/spine-cli/cmd/spine@preview` and follow the [HTTP v3 migration guide](docs/migration/http-v3.md). `@preview` follows a moving branch. Pin a published CLI tag for reproducible installs, and check `spine version` and the generated `go.mod`.

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@latest
spine version
spine new todo-api --module github.com/acme/todo-api
cd todo-api
spine dev
```

To update the CLI itself, rerun the same `go install` command. `spine update` updates the project’s Spine and Echo platform dependencies; it does not update the CLI executable.

In another terminal, check the default health endpoint. It returns `200 OK` and `{"status":"ok"}`.

```sh
curl http://localhost:8080/health
```

By default, `new` configures the Echo platform, installs dependencies, checks the static contract, and builds the app before creating the project. Start the server with `spine dev`. If you use `--skip-install` to generate files only, run `go mod tidy` and `go mod download all` afterward. If static inspection reports `MODULE_GRAPH_UNAVAILABLE`, use the same commands to prepare the full module metadata before checking again.

Run contract and local checks from the project directory.

```sh
spine context --root . --json
spine contract check --root . --warnings-as-errors --json
spine verify --root . --warnings-as-errors --json
```

`spine verify` checks the static contract, formatting, `go vet`, and Go tests. Validate database connections, message brokers, and deployed behavior separately.

Generators refuse to overwrite existing files by default. Review the dry-run changes and `base_hash`, then pass the same hash to `--if-match`.

```sh
spine g resource order --root . --dry-run --json
spine g resource order --root . --if-match <base_hash> --json
```

The CLI analyzes HTTP, DI, interceptor, consumer, and WebSocket registrations, compares contract snapshots, and exports OpenAPI/AsyncAPI. See the [migration guide](docs/migration/http-adapter-modules.md) for manual platform integration and App APIs, and the [CLI documentation](https://github.com/NARUBROWN/spine-cli#readme) for commands and analysis boundaries.


---

## Support

<p align="center">
  <a href="https://fossforall.org/">
    <img src="assets/foss-for-all/logo.png" alt="FOSS for All" width="96" height="96" />
  </a>
</p>

**Spine has been selected as a recipient of [FOSS for All Microgrants 2026](https://fossforall.org/docs/microgrants/2026/#spine).**

[FOSS for All](https://fossforall.org/) supports the development and operation of projects and communities
to help sustain the free and open source ecosystem.
Thank you to FOSS for All and its donors for supporting Spine.

<sub>Logo source: <a href="https://fossforall.org/android-chrome-512x512.png">the official FOSS for All website</a>.
The FOSS for All logo is copyright © FOSS for All and is not covered by Spine's MIT license.
<a href="assets/foss-for-all/README.md">Logo source and attribution</a>.</sub>

---

## License

MIT

## Declare routes inside controllers

The current `v0.7.0-preview.6` source candidate adds `api.Controller.RegisterRoutes` and `input.Path[T]`, `input.Query[T]`, `input.Body[T]`. Register the concrete controller prototype with `app.Constructor(constructor.Controller(&UserController{}))`; a separate route table is unnecessary. This describes the source candidate, not a published tag or CLI release. See the [controller example and description lifecycle](docs/migration/controller-routes.md).

```go
package controller

import (
    "github.com/NARUBROWN/spine/pkg/api"
    "github.com/NARUBROWN/spine/pkg/httpx"
    "github.com/NARUBROWN/spine/pkg/input"
)

type UserController struct{}
type CreateUser struct { Name string `json:"name"` }
type User struct { Name string `json:"name"` }

func (*UserController) Constructor() *UserController { return &UserController{} }


func (c *UserController) RegisterRoutes(r *api.Registry) {
    r.POST("/users", func(a input.Body[CreateUser]) httpx.Response[User] {
        return httpx.Response[User]{Body: User{Name: a.Name}}
    })
}
```
