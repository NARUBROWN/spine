<p align="center">
  <a href="README.md">한국어</a> | <a href="README.en.md">English</a> | 日本語 | <a href="README.zh-Hans.md">简体中文</a> | <a href="README.zh-Hant.md">繁體中文</a>
</p>

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

## 推奨環境

- 推奨Goバージョン: **1.27.3**

## リンク

- 公式サイト: https://spine.na2ru2.me/ja/
- Spine CLI（プロジェクト生成・実行契約の検査）: https://github.com/NARUBROWN/spine-cli
- Bun ORM + Swagger統合サンプルプロジェクト: https://github.com/NARUBROWN/spine-user-demo
- Kafka MSAサンプルプロジェクト: https://github.com/NARUBROWN/spine-simple-msa-demo
- シンプルなWebSocketチャットのサンプルプロジェクト: https://github.com/NARUBROWN/spine-simple-chat-demo

---

## Spine CLI

[Spine CLI](https://github.com/NARUBROWN/spine-cli)は、Spineのプロジェクトやコンポーネントを生成し、
ソースコードからHTTP・DI・インターセプター・コンシューマー・WebSocketの実行契約を検査するGo CLIです。
契約のスナップショット／差分比較や、OpenAPI・AsyncAPIのエクスポートにも対応しています。

Go 1.27.3の使用を推奨します。短い`spine`コマンドは次のようにインストールします。

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@v0.1.4
spine version
```

新しいプロジェクトを作成するか、既存プロジェクトの契約を検査します。

```sh
spine new todo-api --module github.com/acme/todo-api
spine context --root ./todo-api --json
spine contract check --root ./todo-api --warnings-as-errors --json
spine verify --root ./todo-api --warnings-as-errors --json
```

ジェネレーターは、デフォルトでは既存ファイルを上書きしません。変更を適用する前にdry-runの差分と
`base_hash`を確認し、同じハッシュを`--if-match`に渡してください。

```sh
spine g resource order --root . --dry-run --json
spine g resource order --root . --if-match <base_hash> --json
```

Spine CLI `v0.1.4`が新規プロジェクトに指定するSpineのバージョンは`v0.5.1`です。このリポジトリの
別バージョンや開発スナップショットと併用する場合は、生成された`go.mod`と契約検査結果を確認してください。
すべてのコマンドと正確な解析範囲については、
[Spine CLIドキュメント](https://github.com/NARUBROWN/spine-cli#readme)を参照してください。

---

## 支援

<p align="center">
  <a href="https://fossforall.org/">
    <img src="assets/foss-for-all/logo.png" alt="FOSS for All" width="96" height="96" />
  </a>
</p>

**Spineは[FOSS for All Microgrants 2026](https://fossforall.org/docs/microgrants/2026/#spine)の支援対象プロジェクトに選定されました。**

[FOSS for All](https://fossforall.org/)は、持続可能な自由・オープンソースのエコシステムに向けて、
プロジェクトやコミュニティの開発と運営を支援しています。Micrograntsは、取引先への直接支払いや
適切な証憑の確認後の払い戻しによって運営費を支援する小規模な資金援助プログラムです。
Spineを支援してくださるFOSS for Allと寄付者の皆さまに感謝します。

<sub>ロゴ出典：<a href="https://fossforall.org/android-chrome-512x512.png">FOSS for All公式サイト</a>。
FOSS for Allロゴの著作権はFOSS for Allに帰属します（© FOSS for All）。
このロゴはSpineのMITライセンスの対象には含まれません。
<a href="assets/foss-for-all/README.md">ロゴの出典と権利表記</a>。</sub>

---

## ライセンス

MIT
