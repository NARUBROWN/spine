<p align="center">
  <a href="README.md">한국어</a> | English | <a href="README.ja.md">日本語</a> | <a href="README.zh-Hans.md">简体中文</a> | <a href="README.zh-Hant.md">繁體中文</a>
</p>

# 🦴 Spine
<p align="center">
  <img src="assets/spine-logo.png" alt="Spine" width="420" />
</p>

**Spine is a backend framework that makes request execution explicit.**  
It does not hide how a request is resolved and executed.

## Current channel

**RC** | Published core tag [`v0.7.1-rc.1`](https://github.com/NARUBROWN/spine/tree/v0.7.1-rc.1)

Other channels | [Preview](https://github.com/NARUBROWN/spine/tree/preview) | [Stable](https://github.com/NARUBROWN/spine/tree/main)

## Core ideas

- Fixed execution order and responsibilities
- Method signature is the API contract
- Semantic input types: `path.*`, `query.*`, `httperr.*`

## Recommended environment

- Recommended Go version: **1.27.3**

## Get started with Spine CLI

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@v0.4.3-rc.3
spine version
spine new todo-api --module github.com/acme/todo-api --channel rc --no-interactive
cd todo-api
go get github.com/NARUBROWN/spine@v0.7.1-rc.1 github.com/NARUBROWN/spine-echo-platform@v0.2.0-rc.1
go mod tidy
spine dev
```

```sh
curl http://localhost:8080/health
```

Check the generated `go.mod` for the core and platform versions used by the app.

Use Stable for production. `--channel rc` selects the latest published RC or stable tag, so the generated app may use a stable core. The `go get` command above pins the core and Echo platform to the RC tags under test.

## Links

- Official site: https://spine.na2ru2.me/en/
- Spine CLI (project generation and execution-contract inspection): https://github.com/NARUBROWN/spine-cli
- Bun ORM + Swagger integration example project: https://github.com/NARUBROWN/spine-user-demo
- Kafka MSA example project: https://github.com/NARUBROWN/spine-simple-msa-demo
- Simple WebSocket chat example project: https://github.com/NARUBROWN/spine-simple-chat-demo

---

See the [controller route registration example](docs/migration/controller-routes.md) for Spine `v0.7.1`.

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
