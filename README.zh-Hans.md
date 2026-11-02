<p align="center">
  <a href="README.md">한국어</a> Spine `v0.6.0` 提供公开的 HTTP 执行契约，并将 Echo 实现拆分为独立的 `spine-echo-platform` 模块。HTTP 应用使用 `platform.New()` 创建；`spine.New()` 不选择 HTTP 实现。请参阅[迁移指南](docs/migration/http-adapter-modules.md)和[最小示例](cmd/demo/README.md)。
</p>

> 尚未发布的 issue #9 开发分支在 Spine 内管理公开 HTTP 契约，并将 Echo 集成作为独立平台模块提供。HTTP 应用使用 `platform.New()`；内核的 `spine.New()` 不会自动选择 HTTP 实现。请参阅[迁移指南](docs/migration/http-adapter-modules.md)和[本地示例](cmd/demo/README.md)。


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

## 使用 Echo 平台运行应用

选择平台模块，然后调用 Spine App 的 Run。

```sh
go get github.com/NARUBROWN/spine-echo-platform@v0.1.0
```

```go
package main

import (
    "log"

    platform "github.com/NARUBROWN/spine-echo-platform"
    "github.com/NARUBROWN/spine/pkg/boot"
)

func main() {
    app := platform.New()
    if err := app.Run(boot.Options{Address: ":8080", HTTP: &boot.HTTPOptions{}}); err != nil {
        log.Fatal(err)
    }
}
```

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
