<p align="center">
  <a href="README.md">한국어</a> | <a href="README.en.md">English</a> | 日本語 | <a href="README.zh-Hans.md">简体中文</a> | <a href="README.zh-Hant.md">繁體中文</a>
</p>

> **main は v0.6.6 を準備中です。** WebSocket の互換エイリアスを `adapter/websocket` に移動しました。タグとリリースはまだ公開していません。[import の移行ガイド](docs/migration/v0.6.6.md)を参照してください。

> **v0.6.5** は HTTP・WebSocket・event・validator の公開契約と独立した実装選択を統合します。コアは標準ライブラリのみを使用します。[v0.6.5 移行ガイド](docs/migration/v0.6.5.md)を参照してください。

> Spine `v0.6.0` 以降、新しいプロジェクトは [Spine CLI の `new`](https://github.com/NARUBROWN/spine-cli#빠른-시작) で作成します。CLI が Echo プラットフォームと依存関係を設定します。既存アプリは[移行ガイド](docs/migration/http-adapter-modules.md)を参照してください。
>
> 開発ソースのイベント連携は `spine.WithEventAdapter` と Kafka/RabbitMQ プラットフォームを使用します。旧 boot 設定からの移行は [イベントプラットフォーム移行ガイド](docs/migration/event-platforms.en.md)を参照してください。

> 開発ソースの WebSocket は `contract/websocket` と [Gorilla プラットフォーム](https://github.com/NARUBROWN/spine-gorilla-platform)に分離しました。`spine.New(echoPlatform.Option(), gorillaPlatform.Option())` で両方を選択します。[WS 移行ガイド](docs/migration/websocket-platforms.en.md)を参照してください。

# 🦴 Spine
<p align="center">
  <img src="assets/spine-logo.png" alt="Spine" width="420" />
</p>

**Spineは、リクエストの実行フローを明示するバックエンドフレームワークです。**  
リクエストがどのように解釈され、実行されるのかを隠しません。

## コアコンセプト

- 実行順序と責務をコード構造として固定します
- メソッドシグネチャがAPIの契約です
- `path.*`、`query.*`、`httperr.*`のセマンティック型を使用します

## 企業導入の無償技術サポート

[![Spine enterprise adoption support: free guidance, 24/7 inquiries, and maintainer replies as available](assets/enterprise-adoption/banner.png)](https://github.com/NARUBROWN/spine/issues/new?title=%5BAdoption%20support%5D)

SpringやNestJSで慣れ親しんだビジネスアーキテクチャと開発方法を、Goでも活かせるよう支援します。
Spineの導入を検討中、または利用中の企業や開発チームは、開発者による無償サポートを利用できます。
参加企業数の上限や、一律の支援終了期限は設けません。

- サービスにSpineを適用できるか検討し、コントローラー、サービス、リポジトリの役割と依存関係を確認します。
- 初期設定や関連コードをレビューし、依存性注入（DI）やフレームワークの利用で困っている点を確認します。
- 問題を再現し、原因に応じて回避策や修正方法を検討します。

お問い合わせは24時間365日受け付けます。開発者が対応可能な時間に確認し、応答時間や復旧時間は保証しません。
文章、コード、ログを使った非同期のやり取りを基本とし、追加の議論が必要な場合は打ち合わせの日程を相談します。

本番環境への影響と回避策の有無で優先順位を決めます。
同じ優先順位では、受付順と他の利用者への影響も考慮します。

| お問い合わせの種類 | 判断基準 |
|---|---|
| 緊急 | 本番サービスの主要機能が停止している、またはデータ整合性の問題が発生している場合 |
| 導入を妨げる問題 | ビルド、起動、主要機能のエラーにより導入や開発を進められない場合 |
| 一般 | 使い方や設計の相談、関連コードのレビュー、回避策のあるエラー |
| 改善提案 | 新機能、使いやすさ、ドキュメント、サンプルに関する提案 |

企業側の開発チームが業務機能の実装とサービス運用を担当し、インフラや外部サービスの費用を負担します。
プロジェクト全体の開発、常時運用の代行、企業専用機能の長期開発が必要な場合は、支援範囲を別途相談してください。

**[導入相談と技術サポートへのお問い合わせ](https://github.com/NARUBROWN/spine/issues/new?title=%5BAdoption%20support%5D)**

サービス概要、現在の技術構成、導入予定の範囲、質問をお知らせください。
エラーの場合は、SpineとGoのバージョン、期待する動作と実際の動作、最小限の再現手順を添えてください。
GitHub Issuesは公開されます。機密情報を除いた概要や最小限の再現コードを共有してください。

## 推奨環境

- 推奨Goバージョン: **1.27.3**

## リンク

- 公式サイト: https://spine.na2ru2.me/ja/
- Spine CLI（プロジェクト生成・実行契約の検査）: https://github.com/NARUBROWN/spine-cli
- Bun ORM + Swagger統合サンプルプロジェクト: https://github.com/NARUBROWN/spine-user-demo
- Kafka MSAサンプルプロジェクト: https://github.com/NARUBROWN/spine-simple-msa-demo
- シンプルなWebSocketチャットのサンプルプロジェクト: https://github.com/NARUBROWN/spine-simple-chat-demo

---

## Spine CLI で始める

Spine `v0.6.0` 以降、公式のプロジェクト作成方法は `spine new` です。[Spine CLI のインストールガイド](https://github.com/NARUBROWN/spine-cli#권장-환경과-설치)に従って `spine` コマンドをインストールし、対応バージョンを確認してください。

`new` はインストールした CLI に含まれる検証済みレシピに従い、互換性のある Spine、Echo プラットフォーム、Echo のバージョンを設定します。選択されたバージョンは生成された `go.mod` で確認してください。

プロジェクトを作成し、開発サーバーを起動します。

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@latest
spine version
spine new todo-api --module github.com/acme/todo-api
cd todo-api
spine dev
```

CLI 自体を更新する場合は、同じ `go install` コマンドを再実行します。`spine update` はプロジェクトの Spine と Echo プラットフォームの依存関係を更新するコマンドであり、CLI 実行ファイルは更新しません。

別のターミナルで標準の health endpoint を確認します。`200 OK` と `{"status":"ok"}` が返ります。

```sh
curl http://localhost:8080/health
```

`new` は標準で Echo プラットフォームの設定、依存関係のインストール、静的契約の検査とビルドを完了してからプロジェクトを作成します。サーバーは `spine dev` で起動します。`--skip-install` でファイルだけを生成した場合は、その後に `go mod tidy` と `go mod download all` を実行してください。静的検査で `MODULE_GRAPH_UNAVAILABLE` が表示された場合も、この2つのコマンドでモジュールのメタデータを揃えてから再検査します。

プロジェクトのディレクトリで契約とローカル検査を実行します。

```sh
spine context --root . --json
spine contract check --root . --warnings-as-errors --json
spine verify --root . --warnings-as-errors --json
```

`spine verify` は静的契約、コード書式、`go vet` と Go テストを検査します。データベース、メッセージブローカー、デプロイ環境の動作は別途検証してください。

ジェネレーターは標準で既存ファイルを上書きしません。dry-run の変更内容と `base_hash` を確認し、同じ値を `--if-match` に渡してください。

```sh
spine g resource order --root . --dry-run --json
spine g resource order --root . --if-match <base_hash> --json
```

CLI は HTTP、DI、インターセプター、コンシューマー、WebSocket の登録を解析し、契約スナップショットの比較と OpenAPI/AsyncAPI の出力を提供します。手動のプラットフォーム接続と App API は[移行ガイド](docs/migration/http-adapter-modules.md)、全コマンドと解析範囲は [CLI ドキュメント](https://github.com/NARUBROWN/spine-cli#readme)を参照してください。


---

## 支援

<p align="center">
  <a href="https://fossforall.org/">
    <img src="assets/foss-for-all/logo.png" alt="FOSS for All" width="96" height="96" />
  </a>
</p>

**Spineは[FOSS for All Microgrants 2026](https://fossforall.org/docs/microgrants/2026/#spine)の支援対象プロジェクトに選定されました。**

[FOSS for All](https://fossforall.org/)は、持続可能な自由・オープンソースのエコシステムに向けて、
プロジェクトやコミュニティの開発と運営を支援しています。
Spineを支援してくださるFOSS for Allと寄付者の皆さまに感謝します。

<sub>ロゴ出典：<a href="https://fossforall.org/android-chrome-512x512.png">FOSS for All公式サイト</a>。
FOSS for Allロゴの著作権はFOSS for Allに帰属します（© FOSS for All）。
このロゴはSpineのMITライセンスの対象には含まれません。
<a href="assets/foss-for-all/README.md">ロゴの出典と権利表記</a>。</sub>

---

## ライセンス

MIT
