<p align="center">
  <a href="README.md">한국어</a> | <a href="README.en.md">English</a> | <a href="README.ja.md">日本語</a> | <a href="README.zh-Hans.md">简体中文</a> | 繁體中文
</p>

# 🦴 Spine
<p align="center">
  <img src="assets/spine-logo.png" alt="Spine" width="420" />
</p>

**Spine 是一個讓請求執行流程清楚可見的後端框架。**  
它不會隱藏請求如何解析與執行。

## 核心理念

- 透過程式碼結構固定執行順序與職責
- 方法簽章即為 API 契約
- 使用 `path.*`、`query.*`、`httperr.*` 語意型別

## 建議環境

- 建議 Go 版本：**1.27.3**

## 連結

- 官方網站：https://spine.na2ru2.me/zh-Hant/
- Spine CLI（專案產生與執行契約檢查）：https://github.com/NARUBROWN/spine-cli
- Bun ORM + Swagger 整合範例專案：https://github.com/NARUBROWN/spine-user-demo
- Kafka MSA 範例專案：https://github.com/NARUBROWN/spine-simple-msa-demo
- 簡易 WebSocket 聊天範例專案：https://github.com/NARUBROWN/spine-simple-chat-demo

---

## Spine CLI

[Spine CLI](https://github.com/NARUBROWN/spine-cli) 是一個 Go CLI，用來產生 Spine 專案與元件，
並從原始碼檢查 HTTP、DI、攔截器、消費者及 WebSocket 的執行契約。它也支援契約快照與差異比較，
以及匯出 OpenAPI 和 AsyncAPI 文件。

建議使用 Go 1.27.3。使用以下指令安裝簡短的 `spine` 命令。

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@v0.1.4
spine version
```

建立新專案或檢查現有專案的契約。

```sh
spine new todo-api --module github.com/acme/todo-api
spine context --root ./todo-api --json
spine contract check --root ./todo-api --warnings-as-errors --json
spine verify --root ./todo-api --warnings-as-errors --json
```

產生器預設不會覆寫現有檔案。套用變更前，請檢查 dry-run 的差異與 `base_hash`，
再將相同的雜湊值傳給 `--if-match`。

```sh
spine g resource order --root . --dry-run --json
spine g resource order --root . --if-match <base_hash> --json
```

Spine CLI `v0.1.4` 會將新產生專案的 Spine 版本固定為 `v0.5.1`。搭配本儲存庫的其他版本或開發快照
使用時，請檢查產生的 `go.mod` 與契約檢查結果。完整命令與精確的分析範圍請參閱
[Spine CLI 文件](https://github.com/NARUBROWN/spine-cli#readme)。

---

## 授權

MIT
