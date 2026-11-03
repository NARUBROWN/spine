<p align="center">
  한국어 | <a href="README.en.md">English</a> | <a href="README.ja.md">日本語</a> | <a href="README.zh-Hans.md">简体中文</a> | <a href="README.zh-Hant.md">繁體中文</a>
</p>

> **릴리스 채널: Preview → RC → Stable.** 이 소스의 대상 버전은 `v0.6.6`입니다. [출시 절차](RELEASING.md)와 [경로 변경 안내](docs/migration/v0.6.6.md)를 확인하고, 공개된 버전은 [Releases](https://github.com/NARUBROWN/spine/releases)에서 확인하세요.

> **v0.6.5**는 HTTP·WebSocket·event·validator의 공개 계약과 독립 구현 선택을 하나로 통합합니다. 코어는 표준 라이브러리만 사용합니다. [v0.6.5 전환 안내](docs/migration/v0.6.5.md)를 참고하세요.

> Spine `v0.6.0`부터 새 프로젝트는 [Spine CLI의 `new`](https://github.com/NARUBROWN/spine-cli#빠른-시작)로 시작합니다. Echo 플랫폼과 필요한 의존성은 CLI가 구성합니다. 기존 앱은 [전환 안내](docs/migration/http-adapter-modules.md)를 참고하세요.
>
> 개발 소스의 이벤트 연동은 `spine.WithEventAdapter`와 Kafka·RabbitMQ 플랫폼을 사용합니다. 기존 boot 옵션은 [이벤트 플랫폼 전환 안내](docs/migration/event-platforms.md)를 따라 옮기세요.

> 개발 소스의 WebSocket은 `contract/websocket`과 [Gorilla 플랫폼](https://github.com/NARUBROWN/spine-gorilla-platform)으로 분리했습니다. WS 앱은 `spine.New(echoPlatform.Option(), gorillaPlatform.Option())`로 두 구현을 선택합니다. [WS 전환 안내](docs/migration/websocket-platforms.md)를 참고하세요.

# 🦴 Spine

<p align="center">
  <img src="assets/spine-logo.png" alt="Spine" width="420" />
</p>

**Spine은 요청을 해석하고 실행하는 흐름을 명시적으로 구성하는 Go 백엔드 프레임워크입니다.**

## 핵심 아이디어

- 요청 해석, 핸들러 실행, 반환 처리의 책임을 분리합니다.
- 컨트롤러의 메서드 시그니처로 요청 입력과 응답 형식을 정의합니다.
- `path.*`, `query.*`, `httperr.*`로 경로 매개변수, 쿼리, HTTP 오류의 의미를 표현합니다.

## 기업 도입 무상 기술 지원

[![Spine enterprise adoption support: free guidance, 24/7 inquiries, and maintainer replies as available](assets/enterprise-adoption/banner.png)](https://github.com/NARUBROWN/spine/issues/new?title=%5BAdoption%20support%5D)

Spring이나 NestJS에서 익숙한 비즈니스 아키텍처와 개발 방식을 Go에서도 이어갈 수 있도록 돕습니다.
Spine 도입을 검토하거나 사용 중인 기업과 개발팀은 누구나 무상 기술 지원을 신청할 수 있습니다.
참여 기업 수나 지원 종료 기간에 일률적인 제한을 두지 않습니다.

- 서비스에 Spine을 적용할 수 있는지 검토하고, 컨트롤러와 서비스, 저장소의 역할과 의존성 구성을 함께 살펴봅니다.
- 초기 설정과 관련 코드를 검토하고, 의존성 주입(DI)이나 프레임워크 사용 중 막힌 부분을 확인합니다.
- 오류를 재현하고, 원인에 따라 우회 방법이나 수정 방안을 검토합니다.

문의는 24/7 접수합니다. 개발자가 대응 가능한 시간에 확인하며, 응답 시간이나 복구 시간을 보장하지는 않습니다.
글과 코드, 로그를 주고받으며 진행하고, 추가 논의가 필요하면 미팅 일정을 협의합니다.

문의는 운영에 미치는 영향과 우회 방법의 유무를 기준으로 분류합니다.
같은 우선순위에서는 접수 순서와 다른 사용자에게 미치는 영향도 고려합니다.

| 문의 유형 | 판단 기준 |
|---|---|
| 긴급 | 운영 서비스의 핵심 기능이 중단됐거나 데이터 정합성 문제가 발생하고 있는 경우 |
| 적용 차단 | 빌드나 기동, 핵심 기능의 오류로 도입 또는 개발을 진행할 수 없는 경우 |
| 일반 | 사용법이나 설계에 관한 질문, 관련 코드 검토, 우회 방법이 있는 오류 |
| 개선 제안 | 새로운 기능이나 사용성, 문서, 예제에 관한 제안 |

기업 개발팀은 업무 기능 개발과 서비스 운영을 담당하며, 인프라와 외부 서비스 사용료를 부담합니다.
프로젝트 전체의 개발이나 상시 운영 대행, 기업 전용 기능의 장기 개발이 필요하면 지원 범위를 별도로 협의해야 합니다.

**[도입 상담 및 기술 지원 문의하기](https://github.com/NARUBROWN/spine/issues/new?title=%5BAdoption%20support%5D)**

서비스 개요와 현재 기술 구성, 적용하려는 범위, 궁금한 점을 알려주세요.
오류 문의에는 Spine과 Go 버전, 기대한 동작과 실제 동작, 최소 재현 방법을 함께 적어주세요.
GitHub Issues는 공개되므로 비밀정보를 제외한 요약이나 최소 재현 코드를 공유해주세요.

## 권장 환경

- 권장 Go 버전: **1.27.3**

## Spine CLI로 시작하기

Spine `v0.6.0`부터 공식 프로젝트 생성 경로는 `spine new`입니다. [Spine CLI 설치 안내](https://github.com/NARUBROWN/spine-cli#권장-환경과-설치)에서 `spine` 명령을 설치한 뒤 지원 버전을 확인하세요.

`new`는 설치한 CLI에 포함된 검증 레시피를 따라 Spine, Echo 플랫폼과 Echo의 호환 버전 조합을 구성합니다. 선택된 버전은 생성된 `go.mod`에서 확인합니다.

프로젝트를 생성하고 개발 서버를 실행합니다.

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@latest
spine version
spine new todo-api --module github.com/acme/todo-api
cd todo-api
spine dev
```

CLI 자체를 갱신할 때는 같은 `go install` 명령을 다시 실행합니다. `spine update`는 프로젝트의 Spine과 Echo 플랫폼 의존성을 갱신하며 CLI 실행 파일을 갱신하지 않습니다.

다른 터미널에서 기본 health endpoint를 확인합니다. `200 OK`와 `{"status":"ok"}`가 반환됩니다.

```sh
curl http://localhost:8080/health
```

`new`는 기본으로 Echo 플랫폼 연결, 의존성 설치, 정적 계약 검사와 빌드를 완료한 뒤 프로젝트를 생성합니다. 서버는 `spine dev`로 시작합니다. 파일만 생성하는 `--skip-install`을 사용한 경우에는 생성 후 `go mod tidy`와 `go mod download all`을 실행하세요. 정적 검사에서 `MODULE_GRAPH_UNAVAILABLE`이 발생한 경우에도 두 명령으로 전체 모듈 메타데이터를 준비한 뒤 다시 검사합니다.

실행 계약과 로컬 검사는 프로젝트 디렉터리에서 수행합니다.

```sh
spine context --root . --json
spine contract check --root . --warnings-as-errors --json
spine verify --root . --warnings-as-errors --json
```

`spine verify`는 정적 계약, 코드 서식, `go vet`와 Go 테스트를 검사합니다. 데이터베이스, 메시지 브로커와 배포 환경의 동작은 별도로 검증합니다.

생성기는 기본으로 기존 파일을 덮어쓰지 않습니다. 먼저 dry-run의 변경 내용과 `base_hash`를 검토한 뒤 같은 값을 `--if-match`에 넣어 적용합니다.

```sh
spine g resource order --root . --dry-run --json
spine g resource order --root . --if-match <base_hash> --json
```

CLI는 HTTP, DI, 인터셉터, 메시지 소비자와 WebSocket 등록을 분석하고 계약 스냅샷 비교와 OpenAPI/AsyncAPI 내보내기를 지원합니다. 수동 플랫폼 연결과 App API는 [전환 안내](docs/migration/http-adapter-modules.md), 전체 명령과 분석 범위는 [CLI 문서](https://github.com/NARUBROWN/spine-cli#readme)를 참고하세요.

개발 브랜치에서는 쿼리와 헤더를 각각 일반 구조체 포인터로 받도록 전환합니다. 기존 `query.Values`, `header.Values` 사용자는 [구조체 입력 전환 안내](docs/migration/structured-inputs.md)를 참고하세요. 정식 전환 버전은 아직 정해지지 않았습니다.

## 문서와 예제

- [공식 사이트](https://spine.na2ru2.me/ko/)
- [Spine CLI](https://github.com/NARUBROWN/spine-cli): 프로젝트 생성과 실행 계약 검사
- [사용자 API 예제](https://github.com/NARUBROWN/spine-user-demo): Bun ORM과 Swagger 통합
- [Kafka MSA 예제](https://github.com/NARUBROWN/spine-simple-msa-demo): 주문 서버와 재고 워커의 이벤트 처리
- [WebSocket 채팅 예제](https://github.com/NARUBROWN/spine-simple-chat-demo): 인증과 방별 메시지 전송

## 지원

<p align="center">
  <a href="https://fossforall.org/">
    <img src="assets/foss-for-all/logo.png" alt="FOSS for All" width="96" height="96" />
  </a>
</p>

**Spine은 [FOSS for All Microgrants 2026](https://fossforall.org/docs/microgrants/2026/#spine)의 수혜 프로젝트로 선정되었습니다.**

[FOSS for All(포스포올)](https://fossforall.org/)은 자유 소프트웨어와 오픈소스 생태계의 지속 가능한 발전을 지원합니다.
Spine을 지원해 주신 FOSS for All과 기부자 여러분께 감사드립니다.

<sub>로고 출처: <a href="https://fossforall.org/android-chrome-512x512.png">FOSS for All 공식 사이트</a>.
FOSS for All 로고의 저작권은 FOSS for All에 있습니다(© FOSS for All).
이 로고는 Spine의 MIT 라이선스 적용 대상에 포함되지 않습니다.
<a href="assets/foss-for-all/README.md">로고 출처 및 권리 표기</a>.</sub>

## 라이선스

MIT
