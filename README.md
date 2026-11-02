<p align="center">
  한국어 | <a href="README.en.md">English</a> Spine `v0.6.0`은 공개 HTTP 실행 계약을 제공하고 Echo 구현을 별도 `spine-echo-platform` 모듈로 분리합니다. HTTP 앱은 `platform.New()`로 생성하며, `spine.New()`는 HTTP 구현을 선택하지 않습니다. [전환 안내](docs/migration/http-adapter-modules.md)와 [최소 예제](cmd/demo/README.md)를 참고하세요.
</p>

> 이슈 #9의 개발 브랜치에서는 공개 실행 계약은 Spine에서 관리하고 Echo 통합은 별도 플랫폼 모듈로 분리합니다. HTTP 앱은 `platform.New()`로 생성하며, 커널의 `spine.New()`는 HTTP 구현을 자동 선택하지 않습니다. 아직 공개 릴리스 전이며 [전환 안내](docs/migration/http-adapter-modules.md)와 [로컬 예제](cmd/demo/README.md)를 참고하세요.


# 🦴 Spine

<p align="center">
  <img src="assets/spine-logo.png" alt="Spine" width="420" />
</p>

**Spine은 요청을 해석하고 실행하는 흐름을 명시적으로 구성하는 Go 백엔드 프레임워크입니다.**

## 핵심 아이디어

- 요청 해석, 핸들러 실행, 반환 처리의 책임을 분리합니다.
- 컨트롤러의 메서드 시그니처로 요청 입력과 응답 형식을 정의합니다.
- `path.*`, `query.*`, `httperr.*`로 경로 매개변수, 쿼리, HTTP 오류의 의미를 표현합니다.

## 권장 환경

- 권장 Go 버전: **1.27.3**

## Echo 플랫폼으로 앱 실행

HTTP 앱은 플랫폼 모듈을 선택하고 Spine App의 Run을 호출합니다.

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

[Spine CLI](https://github.com/NARUBROWN/spine-cli)로 프로젝트와 구성 요소를 생성하고 실행 계약을 검사할 수 있습니다.
HTTP 라우트, 의존성 주입(DI), 인터셉터, 메시지 소비자, WebSocket의 등록 정보를 소스 코드에서 분석합니다.
계약 스냅샷 저장과 비교, OpenAPI와 AsyncAPI 문서 내보내기도 지원합니다.

### 설치

다음 명령으로 CLI를 설치하고 버전을 확인합니다.

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@v0.1.4
spine version
```

### 프로젝트 생성과 검증

프로젝트를 만든 뒤 의존성을 정리하고 실행 계약을 검사합니다.

```sh
spine new todo-api --module github.com/acme/todo-api
go -C ./todo-api mod tidy
spine context --root ./todo-api --json
spine contract check --root ./todo-api --warnings-as-errors --json
spine verify --root ./todo-api --warnings-as-errors --json
```

`spine verify`가 성공하면 실행 계약, 코드 서식, `go vet`, Go 테스트 검사를 통과한 것입니다.
실제 데이터베이스나 메시지 브로커 연결과 배포 환경의 동작은 별도로 검증해야 합니다.

### 기존 파일 변경 전 확인

생성기는 기본적으로 기존 파일을 덮어쓰지 않습니다. 먼저 dry-run으로 변경 내용을 확인하고,
출력의 `base_hash` 값을 아래 `<base_hash>`에 넣어 적용합니다.

```sh
spine g resource order --root . --dry-run --json
spine g resource order --root . --if-match <base_hash> --json
```

CLI `v0.1.4`가 생성하는 프로젝트는 Spine `v0.5.1`을 사용합니다.
다른 Spine 버전이나 이 저장소의 개발 버전과 함께 사용할 때는 생성된 `go.mod`와 계약 검사 결과를 확인하세요.
전체 명령과 분석 범위는 [Spine CLI 문서](https://github.com/NARUBROWN/spine-cli#readme)를 참고하세요.

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
