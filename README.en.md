<p align="center">
  <a href="README.md">한국어</a> | English | <a href="README.ja.md">日本語</a> | <a href="README.zh-Hans.md">简体中文</a> | <a href="README.zh-Hant.md">繁體中文</a>
</p>

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

## Recommended environment

- Recommended Go version: **1.27.3**

## Links

- Official site: https://spine.na2ru2.me/en/
- Spine CLI (project generation and execution-contract inspection): https://github.com/NARUBROWN/spine-cli
- Bun ORM + Swagger integration example project: https://github.com/NARUBROWN/spine-user-demo
- Kafka MSA example project: https://github.com/NARUBROWN/spine-simple-msa-demo
- Simple WebSocket chat example project: https://github.com/NARUBROWN/spine-simple-chat-demo

---

## Spine CLI

[Spine CLI](https://github.com/NARUBROWN/spine-cli) is a Go CLI that generates Spine projects and components
and inspects HTTP, DI, interceptor, consumer, and WebSocket execution contracts from source code. It also
supports contract snapshots and diffs, plus OpenAPI and AsyncAPI exports.

Go 1.27.3 is recommended. Install the short `spine` command as follows.

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@v0.1.4
spine version
```

Create a project or inspect the contract of an existing one.

```sh
spine new todo-api --module github.com/acme/todo-api
spine context --root ./todo-api --json
spine contract check --root ./todo-api --warnings-as-errors --json
spine verify --root ./todo-api --warnings-as-errors --json
```

The generator does not overwrite existing files by default. Before applying a change, review the dry-run diff
and `base_hash`, then pass that same hash to `--if-match`.

```sh
spine g resource order --root . --dry-run --json
spine g resource order --root . --if-match <base_hash> --json
```

Spine CLI `v0.1.4` pins newly generated projects to Spine `v0.5.1`. When using it with another version or a
development snapshot of this repository, check the generated `go.mod` and the contract results. See the
[Spine CLI documentation](https://github.com/NARUBROWN/spine-cli#readme) for the complete command set and
precise analysis boundaries.

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
