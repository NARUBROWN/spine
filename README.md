<p align="center">
  한국어 | <a href="README.en.md">English</a> | <a href="README.ja.md">日本語</a> | <a href="README.zh-Hans.md">简体中文</a> | <a href="README.zh-Hant.md">繁體中文</a>
</p>

# 🦴 Spine
<p align="center">
  <img src="assets/spine-logo.png" alt="Spine" width="420" />
</p>

**Spine는 실행 흐름이 드러나는 백엔드 프레임워크입니다.**  
요청이 어떻게 해석되고 실행되는지 숨기지 않습니다.

## 핵심 아이디어

- 실행 순서와 책임을 코드 구조로 고정합니다
- 메서드 시그니처가 API 계약입니다
- `path.*`, `query.*`, `httperr.*` 의미 타입을 사용합니다

## 요구사항

- Go **1.27.3 이상**. Go 1.27.x 검증 기준은 1.27.3입니다.

## 링크

- 공식 사이트: https://spine.na2ru2.me/ko/
- Spine CLI (프로젝트 생성 및 실행 계약 검사): https://github.com/NARUBROWN/spine-cli
- Bun ORM + Swagger 통합 예제 프로젝트: https://github.com/NARUBROWN/spine-user-demo
- Kafka MSA 예제 프로젝트: https://github.com/NARUBROWN/spine-simple-msa-demo
- 간단한 WebSocket 채팅 예제 프로젝트: https://github.com/NARUBROWN/spine-simple-chat-demo

---

## Spine CLI

[Spine CLI](https://github.com/NARUBROWN/spine-cli)는 Spine 프로젝트와 구성 요소를 생성하고,
소스 코드에서 HTTP·DI·interceptor·consumer·WebSocket 실행 계약을 검사하는 Go CLI입니다.
계약 snapshot/diff와 OpenAPI·AsyncAPI 내보내기도 지원합니다.

Go 1.27.3 이상에서 짧은 `spine` 명령으로 설치할 수 있습니다.

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@v0.1.4
spine version
```

새 프로젝트를 만들거나 기존 프로젝트의 계약을 검사합니다.

```sh
spine new todo-api --module github.com/acme/todo-api
spine context --root ./todo-api --json
spine contract check --root ./todo-api --warnings-as-errors --json
spine verify --root ./todo-api --warnings-as-errors --json
```

생성기는 기존 파일을 기본적으로 덮어쓰지 않습니다. 변경 전에는 dry-run의 diff와
`base_hash`를 검토한 뒤 같은 hash를 `--if-match`로 전달하세요.

```sh
spine g resource order --root . --dry-run --json
spine g resource order --root . --if-match <base_hash> --json
```

현재 CLI `v0.1.4`가 새 프로젝트에 지정하는 Spine 버전은 `v0.5.1`입니다. 이 저장소의 다른
버전이나 개발 snapshot과 함께 사용할 때는 생성된 `go.mod`와 계약 검사 결과를 확인하세요.
전체 명령과 정확한 분석 범위는 [Spine CLI 문서](https://github.com/NARUBROWN/spine-cli#readme)를
참고하세요.

---

## License

MIT
