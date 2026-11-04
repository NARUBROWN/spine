<p align="center">
  <a href="README.md">한국어</a> | <a href="README.en.md">English</a> | <a href="README.ja.md">日本語</a> | <a href="README.zh-Hans.md">简体中文</a> | 繁體中文
</p>

> **發布管道：Preview → RC → Stable。** 此原始碼的目標版本為 `v0.7.0`。請參閱[發布流程](RELEASING.md)、[import 遷移指南](docs/migration/v0.7.0.md)和[已發布版本](https://github.com/NARUBROWN/spine/releases)。

> **v0.6.5** 整合 HTTP、WebSocket、event、validator 的公開契約與獨立實作選擇。核心僅使用標準函式庫。請參閱 [v0.6.5 遷移指南](docs/migration/v0.6.5.md)。

> 從 Spine `v0.6.0` 開始，使用 [Spine CLI 的 `new`](https://github.com/NARUBROWN/spine-cli#빠른-시작) 建立新專案。CLI 會設定 Echo 平台和相依套件。既有應用請參閱[遷移指南](docs/migration/http-adapter-modules.md)。
>
> 開發原始碼透過 `spine.WithEventAdapter` 接入 Kafka/RabbitMQ 平台。舊 broker boot 設定請依[事件平台遷移指南](docs/migration/event-platforms.en.md)遷移。

> 開發原始碼透過 `contract/websocket` 和 [Gorilla 平台](https://github.com/NARUBROWN/spine-gorilla-platform)分離 WebSocket。使用 `spine.New(echoPlatform.Option(), gorillaPlatform.Option())` 選擇兩種實作。請參閱 [WS 遷移指南](docs/migration/websocket-platforms.en.md)。

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

## 企業導入免費技術支援

[![Spine enterprise adoption support: free guidance, 24/7 inquiries, and maintainer replies as available](assets/enterprise-adoption/banner.png)](https://github.com/NARUBROWN/spine/issues/new?title=%5BAdoption%20support%5D)

協助企業與開發團隊將熟悉的 Spring 或 NestJS 業務架構及開發方式延續到 Go。
正在評估或使用 Spine 的企業與開發團隊，皆可直接向開發者申請免費技術支援。
不限制參與企業的數量，也不設統一的支援終止期限。

- 評估 Spine 是否適合你的服務，並檢視控制器、服務、儲存庫的職責與相依關係。
- 檢視初始設定及相關程式碼，協助處理相依性注入（DI）或框架使用上的問題。
- 重現錯誤，並依原因評估替代方案或修正方法。

全天候受理諮詢。開發者於時間允許時處理，不保證固定回應時間或復原時間。
以文字、程式碼及日誌的非同步溝通為主，需要進一步討論時再協調會議時間。

依正式環境影響及是否有替代方案決定優先順序。
同一優先順序內，也會考量受理順序及對其他使用者的影響。

| 諮詢類型 | 判斷依據 |
|---|---|
| 緊急 | 正式環境服務的核心功能中斷，或正在發生資料一致性問題 |
| 導入受阻 | 建置、啟動或核心功能錯誤導致無法繼續導入或開發 |
| 一般 | 使用或架構問題、相關程式碼審查、已有替代方案的錯誤 |
| 改善建議 | 關於新功能、易用性、文件或範例的建議 |

企業開發團隊負責業務功能開發與服務營運，並負擔基礎設施及第三方服務費用。
整個專案的開發、持續代維運或企業專屬功能的長期開發，需要另行商定支援範圍。

**[諮詢導入方案或尋求技術支援](https://github.com/NARUBROWN/spine/issues/new?title=%5BAdoption%20support%5D)**

請說明服務概況、目前技術組合、預計導入範圍與問題。
回報錯誤時，請提供 Spine 與 Go 版本、預期與實際行為，以及最小重現步驟。
GitHub Issues 為公開內容。請分享已移除機密資訊的摘要或最小重現程式碼。

## 建議環境

- 建議 Go 版本：**1.27.3**

## 連結

- 官方網站：https://spine.na2ru2.me/zh-Hant/
- Spine CLI（專案產生與執行契約檢查）：https://github.com/NARUBROWN/spine-cli
- Bun ORM + Swagger 整合範例專案：https://github.com/NARUBROWN/spine-user-demo
- Kafka MSA 範例專案：https://github.com/NARUBROWN/spine-simple-msa-demo
- 簡易 WebSocket 聊天範例專案：https://github.com/NARUBROWN/spine-simple-chat-demo

---

## 使用 Spine CLI 開始

從 Spine `v0.6.0` 開始，官方專案建立方式是 `spine new`。請依照 [Spine CLI 安裝指南](https://github.com/NARUBROWN/spine-cli#권장-환경과-설치)安裝 `spine` 指令，並確認支援的版本。

`new` 依照已安裝 CLI 內建的驗證配方，設定相容的 Spine、Echo 平台與 Echo 版本組合。請在產生的 `go.mod` 中查看選取的版本。

建立專案並啟動開發伺服器。

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@latest
spine version
spine new todo-api --module github.com/acme/todo-api
cd todo-api
spine dev
```

更新 CLI 本身時，請重新執行相同的 `go install` 指令。`spine update` 更新專案的 Spine 與 Echo 平台相依套件，不會更新 CLI 執行檔。

在另一個終端機中檢查預設的 health endpoint。它應回傳 `200 OK` 和 `{"status":"ok"}`。

```sh
curl http://localhost:8080/health
```

預設情況下，`new` 完成 Echo 平台設定、相依套件安裝、靜態契約檢查與建置後才建立專案。使用 `spine dev` 啟動伺服器。如果使用 `--skip-install` 僅產生檔案，請接著執行 `go mod tidy` 和 `go mod download all`。若靜態檢查回報 `MODULE_GRAPH_UNAVAILABLE`，也應先使用這兩個命令準備完整的模組中繼資料，再重新檢查。

在專案目錄中執行契約與本機檢查。

```sh
spine context --root . --json
spine contract check --root . --warnings-as-errors --json
spine verify --root . --warnings-as-errors --json
```

`spine verify` 檢查靜態契約、程式碼格式、`go vet` 與 Go 測試。資料庫連線、訊息代理與部署環境的行為需要另外驗證。

產生器預設不會覆寫既有檔案。先檢查 dry-run 的變更與 `base_hash`，再將相同值傳給 `--if-match`。

```sh
spine g resource order --root . --dry-run --json
spine g resource order --root . --if-match <base_hash> --json
```

CLI 分析 HTTP、DI、攔截器、消費者與 WebSocket 註冊，支援契約快照比較及 OpenAPI/AsyncAPI 匯出。手動平台整合與 App API 請參閱[遷移指南](docs/migration/http-adapter-modules.md)，完整指令與分析範圍請參閱 [CLI 文件](https://github.com/NARUBROWN/spine-cli#readme)。


---

## 支持

<p align="center">
  <a href="https://fossforall.org/">
    <img src="assets/foss-for-all/logo.png" alt="FOSS for All" width="96" height="96" />
  </a>
</p>

**Spine 已入選 [FOSS for All Microgrants 2026](https://fossforall.org/docs/microgrants/2026/#spine) 資助專案。**

[FOSS for All](https://fossforall.org/) 支持專案與社群的開發及營運，推動自由與開源生態的永續發展。
感謝 FOSS for All 及捐贈者對 Spine 的支持。

<sub>標誌來源：<a href="https://fossforall.org/android-chrome-512x512.png">FOSS for All 官方網站</a>。
FOSS for All 標誌的著作權歸 FOSS for All 所有（© FOSS for All），不屬於 Spine 的 MIT 授權範圍。
<a href="assets/foss-for-all/README.md">標誌來源及著作權說明</a>。</sub>

---

## 授權

MIT
