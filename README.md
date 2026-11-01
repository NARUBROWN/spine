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

## 권장 환경

- 권장 Go 버전: **1.27.3**

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

Go 1.27.3 사용을 권장합니다. 짧은 `spine` 명령은 다음과 같이 설치합니다.

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

## 지원

<p align="center">
  <a href="https://fossforall.org/">
    <img src="assets/foss-for-all/logo.png" alt="FOSS for All" width="96" height="96" />
  </a>
</p>

**Spine은 [FOSS for All Microgrants 2026](https://fossforall.org/docs/microgrants/2026/#spine)의 수혜 프로젝트로 선정되었습니다.**

[FOSS for All(포스포올)](https://fossforall.org/)은 지속 가능한 자유·오픈소스 생태계를 위해
프로젝트와 커뮤니티의 개발과 운영을 지원합니다. Microgrants는 필요한 운영비를 대리 집행하거나
적격 증빙 확인 후 환급하는 소규모 재정 지원 사업입니다.
Spine을 지원해 주신 FOSS for All과 기부자 여러분께 감사드립니다.

<sub>로고 출처: <a href="https://fossforall.org/android-chrome-512x512.png">FOSS for All 공식 사이트</a>.
FOSS for All 로고의 저작권은 FOSS for All에 있습니다(© FOSS for All).
이 로고는 Spine의 MIT 라이선스 적용 대상에 포함되지 않습니다.
<a href="assets/foss-for-all/README.md">로고 출처 및 권리 표기</a>.</sub>

---

## License

MIT
