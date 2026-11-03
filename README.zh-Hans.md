<p align="center">
  <a href="README.md">한국어</a> | <a href="README.en.md">English</a> | <a href="README.ja.md">日本語</a> | 简体中文 | <a href="README.zh-Hant.md">繁體中文</a>
</p>

> 从 Spine `v0.6.0` 开始，使用 [Spine CLI 的 `new`](https://github.com/NARUBROWN/spine-cli#빠른-시작) 创建新项目。CLI 会配置 Echo 平台和依赖。现有应用请参阅[迁移指南](docs/migration/http-adapter-modules.md)。
>
> 开发源码通过 `spine.WithEventAdapter` 接入 Kafka/RabbitMQ 平台。旧 broker boot 配置请按[事件平台迁移指南](docs/migration/event-platforms.en.md)迁移。

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

## 企业采用免费技术支持

[![Spine enterprise adoption support: free guidance, 24/7 inquiries, and maintainer replies as available](assets/enterprise-adoption/banner.png)](https://github.com/NARUBROWN/spine/issues/new?title=%5BAdoption%20support%5D)

帮助企业和开发团队将熟悉的 Spring 或 NestJS 业务架构与开发方式延续到 Go。
正在评估或使用 Spine 的企业和开发团队，均可直接向开发者申请免费技术支持。
不限制参与企业的数量，也不设统一的支持终止期限。

- 评估 Spine 是否适合你的服务，并审查控制器、服务、仓储的职责和依赖关系。
- 审查初始配置与相关代码，协助解决依赖注入（DI）或框架使用中的问题。
- 复现错误，并根据原因评估规避方法或修复方案。

全天候接受咨询。开发者在时间允许时处理，不保证固定响应时间或恢复时间。
以文字、代码和日志的异步沟通为主，需要进一步讨论时再协商会议时间。

根据生产影响和是否存在规避方法确定优先级。
同一优先级内，也会考虑接收顺序及对其他用户的影响。

| 咨询类型 | 判断依据 |
|---|---|
| 紧急 | 生产服务核心功能中断，或正在发生数据一致性问题 |
| 采用受阻 | 构建、启动或核心功能错误导致无法继续采用或开发 |
| 一般 | 使用或架构问题、相关代码审查、存在规避方法的错误 |
| 改进建议 | 关于新功能、易用性、文档或示例的建议 |

企业开发团队负责业务功能开发和服务运营，并承担基础设施与第三方服务费用。
整个项目的开发、持续代运维或企业专属功能的长期开发，需要另行商定支持范围。

**[咨询采用方案或寻求技术支持](https://github.com/NARUBROWN/spine/issues/new?title=%5BAdoption%20support%5D)**

请说明服务概况、当前技术栈、拟采用范围和问题。
报告错误时，请提供 Spine 和 Go 版本、预期与实际行为以及最小复现步骤。
GitHub Issues 为公开内容。请分享已去除机密信息的摘要或最小复现代码。

## 推荐环境

- 推荐 Go 版本：**1.27.3**

## 链接

- 官方网站：https://spine.na2ru2.me/zh-Hans/
- Spine CLI（项目生成与执行契约检查）：https://github.com/NARUBROWN/spine-cli
- Bun ORM + Swagger 集成示例项目：https://github.com/NARUBROWN/spine-user-demo
- Kafka MSA 示例项目：https://github.com/NARUBROWN/spine-simple-msa-demo
- 简单的 WebSocket 聊天示例项目：https://github.com/NARUBROWN/spine-simple-chat-demo

---

## 使用 Spine CLI 开始

从 Spine `v0.6.0` 开始，官方项目创建方式是 `spine new`。请按照 [Spine CLI 安装指南](https://github.com/NARUBROWN/spine-cli#권장-환경과-설치)安装 `spine` 命令，并确认支持的版本。

`new` 根据已安装 CLI 内置的验证配方，配置兼容的 Spine、Echo 平台和 Echo 版本组合。请在生成的 `go.mod` 中查看所选版本。

创建项目并启动开发服务器。

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@latest
spine version
spine new todo-api --module github.com/acme/todo-api
cd todo-api
spine dev
```

更新 CLI 本身时，请重新运行相同的 `go install` 命令。`spine update` 更新项目的 Spine 和 Echo 平台依赖，不会更新 CLI 可执行文件。

在另一个终端中检查默认的 health endpoint。它应返回 `200 OK` 和 `{"status":"ok"}`。

```sh
curl http://localhost:8080/health
```

默认情况下，`new` 完成 Echo 平台配置、依赖安装、静态契约检查和构建后才创建项目。使用 `spine dev` 启动服务器。如果使用 `--skip-install` 仅生成文件，请随后运行 `go mod tidy` 和 `go mod download all`。如果静态检查报告 `MODULE_GRAPH_UNAVAILABLE`，也应先用这两个命令准备完整的模块元数据，再重新检查。

在项目目录中运行契约与本地检查。

```sh
spine context --root . --json
spine contract check --root . --warnings-as-errors --json
spine verify --root . --warnings-as-errors --json
```

`spine verify` 检查静态契约、代码格式、`go vet` 和 Go 测试。数据库连接、消息代理和部署环境的行为需要单独验证。

生成器默认不会覆盖现有文件。先检查 dry-run 的更改和 `base_hash`，再将同一个值传给 `--if-match`。

```sh
spine g resource order --root . --dry-run --json
spine g resource order --root . --if-match <base_hash> --json
```

CLI 分析 HTTP、DI、拦截器、消费者和 WebSocket 注册，支持契约快照比较及 OpenAPI/AsyncAPI 导出。手动平台集成和 App API 请参阅[迁移指南](docs/migration/http-adapter-modules.md)，完整命令与分析范围请参阅 [CLI 文档](https://github.com/NARUBROWN/spine-cli#readme)。


---

## 支持

<p align="center">
  <a href="https://fossforall.org/">
    <img src="assets/foss-for-all/logo.png" alt="FOSS for All" width="96" height="96" />
  </a>
</p>

**Spine 已入选 [FOSS for All Microgrants 2026](https://fossforall.org/docs/microgrants/2026/#spine) 资助项目。**

[FOSS for All](https://fossforall.org/) 支持项目和社区的开发与运营，推动自由及开源生态的可持续发展。
感谢 FOSS for All 及捐赠者对 Spine 的支持。

<sub>标志来源：<a href="https://fossforall.org/android-chrome-512x512.png">FOSS for All 官方网站</a>。
FOSS for All 标志的版权归 FOSS for All 所有（© FOSS for All），不属于 Spine 的 MIT 许可证授权范围。
<a href="assets/foss-for-all/README.md">标志来源及版权说明</a>。</sub>

---

## 许可证

MIT
