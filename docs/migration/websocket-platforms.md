# WebSocket 구현을 독립 플랫폼으로 선택하기

WS를 등록하는 앱은 HTTP와 WebSocket 구현을 각각 선택합니다. 공개 WS 계약은 `contract/websocket`의 `spine.websocket-adapter/v1`이며, Gorilla 구현은 [spine-gorilla-platform](https://github.com/NARUBROWN/spine-gorilla-platform)에 있습니다. 코어 모듈은 표준 라이브러리만 사용합니다.

## 조립 코드

```go
app := spine.New(
    echoPlatform.Option(),
    gorillaPlatform.Option(),
    spine.WithEventAdapter("events", factory),
    spine.WithValidator(validator),
)
```

위 코드는 `spine`, `echoPlatform "github.com/NARUBROWN/spine-echo-platform"`, `gorillaPlatform "github.com/NARUBROWN/spine-gorilla-platform"`을 import한 `BuildApplication`의 생성식입니다. 이벤트 factory는 기존처럼 `kafkaplatform.NewFactory(config)` 또는 RabbitMQ의 `NewFactory`로 생성하고 오류를 반환합니다. validator도 `playground.New()`의 오류를 처리한 뒤 전달합니다. 이벤트·검증기 API와 사용자 생성자·라우트·consumer 등록 방식은 유지합니다.

HTTP만 사용하는 앱은 Gorilla 모듈과 옵션을 생략합니다. HTTP 없는 worker에는 HTTP·WS 옵션을 모두 생략합니다. WS 경로가 있으면 HTTP 플랫폼의 업그레이드 지원과 WS factory가 모두 필요하며, 누락 시 네트워크 초기화 전에 실패합니다.

## 연결 정책

`boot.HTTPOptions.WebSocket`과 `ws.Send`는 그대로 사용합니다. 인증·출처·trusted proxy·연결 수 제한과 정상 종료는 코어가 관리합니다. Gorilla 플랫폼은 업그레이드, 읽기 크기·시간 제한과 제어 프레임을 처리합니다. 읽기 타임아웃은 다음 메시지를 읽기 시작할 때 갱신하므로 핸들러 실행 시간을 포함하지 않습니다.

DTO 규칙 위반은 핸들러 실행 없이 1008, 검증 처리 오류는 1011로 연결을 닫습니다. 정상 종료는 1000 close 프레임을 보내고 읽기·핸들러가 끝나기를 기다립니다. [상세 계약](../../contract/websocket/README.md)을 참고하세요.

## 버전과 CLI

- Spine: `v0.6.1-0.20261103040400-9b1321410211`
- Echo 플랫폼: `v0.1.0`
- Gorilla 플랫폼: `v0.0.0-20261103040512-36ad41dff88f`

위 버전은 공개 Go 모듈로 설치할 수 있는 개발 버전 조합입니다. 코어 PR 병합·정식 태그 릴리스와 구분합니다. 코어와 Gorilla는 Go 1.25.5 이상, 현재 playground 검증기를 함께 선택한 앱은 Go 1.26 이상이 필요합니다.

CLI PR #28의 새 생성기는 `--websocket-platform none|gorilla`를 제공합니다. 기본값은 `none`이며 `gorilla`는 HTTP `echo`와 함께 선택합니다. 설치한 CLI의 `spine schema --json`에서 해당 옵션을 확인하세요. 기존 프로젝트의 `spine update`는 의존성 갱신만 수행하므로 WS 옵션이나 소스를 자동 추가하지 않습니다. 기존 레시피를 새 코어로 직접 올릴 때는 생성식을 먼저 검토해 명시적인 WS 선택을 추가해야 합니다.

## 검증

```sh
go test -race ./... -count=1
bash scripts/check-module-boundaries.sh
go -C test test -race ./... -count=1
go -C cmd/demo test -race ./... -count=1
```

실제 브로커 조합은 [공개 플랫폼 E2E](../../test/platform-e2e/README.md)에서 별도로 실행합니다. CLI 생성 결과의 HTTP·WS·이벤트·검증기 조합은 CLI 저장소의 `test/assembly-e2e/README.md`에 재현 절차가 있습니다. 정적 검사, 로컬 소스 테스트, 공개 버전 설치와 실제 브로커 실행은 각각 구분해 확인합니다.
