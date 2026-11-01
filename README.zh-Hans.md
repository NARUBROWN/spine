<p align="center">
  <a href="README.md">한국어</a> | <a href="README.en.md">English</a> | <a href="README.ja.md">日本語</a> | 简体中文 | <a href="README.zh-Hant.md">繁體中文</a>
</p>

# 🦴 Spine
<p align="center">
  <img src="assets/spine-logo.png" alt="Spine" width="420" />
</p>

**Spine 是一个让请求执行流程清晰可见的后端框架。**  
它不会隐藏请求如何解析和执行。

## 核心理念

- 通过代码结构固定执行顺序和职责
- 方法签名即 API 契约
- 使用 `path.*`、`query.*`、`httperr.*` 语义类型

## 推荐环境

- 推荐 Go 版本：**1.27.3**

## 链接

- 官方网站：https://spine.na2ru2.me/zh-Hans/
- Spine CLI（项目生成与执行契约检查）：https://github.com/NARUBROWN/spine-cli
- Bun ORM + Swagger 集成示例项目：https://github.com/NARUBROWN/spine-user-demo
- Kafka MSA 示例项目：https://github.com/NARUBROWN/spine-simple-msa-demo
- 简单的 WebSocket 聊天示例项目：https://github.com/NARUBROWN/spine-simple-chat-demo

---

## Spine CLI

[Spine CLI](https://github.com/NARUBROWN/spine-cli) 是一个 Go CLI，用于生成 Spine 项目和组件，
并从源代码检查 HTTP、DI、拦截器、消费者和 WebSocket 的执行契约。它还支持契约快照与差异比较，
以及导出 OpenAPI 和 AsyncAPI 文档。

推荐使用 Go 1.27.3。使用以下命令安装简短的 `spine` 命令。

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@v0.1.4
spine version
```

创建新项目或检查现有项目的契约。

```sh
spine new todo-api --module github.com/acme/todo-api
spine context --root ./todo-api --json
spine contract check --root ./todo-api --warnings-as-errors --json
spine verify --root ./todo-api --warnings-as-errors --json
```

生成器默认不会覆盖现有文件。应用更改前，请检查 dry-run 的差异和 `base_hash`，
然后将同一个哈希值传给 `--if-match`。

```sh
spine g resource order --root . --dry-run --json
spine g resource order --root . --if-match <base_hash> --json
```

Spine CLI `v0.1.4` 会将新生成项目的 Spine 版本固定为 `v0.5.1`。与本仓库的其他版本或开发快照
配合使用时，请检查生成的 `go.mod` 和契约检查结果。完整命令和准确的分析边界请参阅
[Spine CLI 文档](https://github.com/NARUBROWN/spine-cli#readme)。

---

## 许可证

MIT
