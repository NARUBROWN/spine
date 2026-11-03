# WebSocket 플랫폼 계약

`github.com/NARUBROWN/spine/contract/websocket`는 Spine과 WebSocket 구현이 공유하는 공개 계약입니다. 계약 버전은 `spine.websocket-adapter/v1`이며 Spine 모듈에 포함됩니다. 표준 라이브러리만 사용합니다.

HTTP 플랫폼은 `httpcontract.UpgradeRoute`를 마운트하고, WS 플랫폼은 해당 요청을 업그레이드한 뒤 `Connection`을 반환합니다. 두 플랫폼은 서로 import하지 않습니다. 커널은 WS 구현을 기본으로 선택하지 않습니다.

## 계약과 책임

| 계약 | 책임 |
|---|---|
| `Factory.Capabilities()` | 지원하는 계약 버전 반환 |
| `Factory.New(Options)` | 네트워크 연결과 실행 작업 없이 어댑터 준비 |
| `Adapter.Upgrade(w, r)` | 검사를 통과한 요청의 업그레이드, 실패 시 HTTP 오류 응답 |
| `Connection.ReadMessage()` | text/binary 메시지 수신, 제어 프레임과 읽기 제한 처리 |
| `Connection.WriteMessage(type, payload, deadline)` | 지정한 절대 시각까지 메시지 전송 |
| `Connection.Ping(deadline)` | ping 전송; pong 대기는 읽기 루프가 처리 |
| `Connection.WriteClose(code, reason, deadline)` | close 프레임 전송 |
| `Connection.Close()` | 연결 해제와 진행 중인 읽기·쓰기 종료 |

`ValidateFactory`는 nil, 인터페이스 안의 nil 포인터, 계약 버전을 검사합니다. `Factory.New`는 호출하지 않습니다. `New`가 반환한 어댑터와 `Upgrade`가 반환한 연결도 nil이면 사용할 수 없습니다. `Upgrade`가 오류와 함께 연결을 반환하면 커널이 그 연결을 닫습니다. 커널은 플랫폼이 작성한 업그레이드 오류 응답을 다시 작성하지 않습니다.

어댑터는 여러 요청의 `Upgrade`를 동시에 처리할 수 있어야 합니다. 커널은 연결당 읽기 루프 하나를 실행하고 메시지·ping·close 쓰기를 직렬화합니다. `Close`는 다른 메서드와 동시에 호출할 수 있어야 합니다.

## 연결 설정

커널은 `boot.HTTPOptions.WebSocket`을 검증하고 기본값을 적용한 뒤 다음 값만 플랫폼에 전달합니다. 플랫폼은 양수가 아닌 값을 거부합니다.

| `Options` 필드 | 코어 기본값 | 의미 |
|---|---|---|
| `HandshakeTimeout` | 10초 | 업그레이드 응답 쓰기 제한 시간 |
| `ReadTimeout` | 60초 | 메시지 또는 pong 수신 대기 시간 |
| `MaxMessageBytes` | 1 MiB | 수신 메시지 최대 크기 |

플랫폼은 읽기를 시작할 때와 pong을 받을 때 읽기 제한 시간을 갱신합니다. 핸들러 실행 시간은 다음 읽기의 제한 시간에서 제외합니다. 크기를 초과한 메시지는 close 코드 1009로 종료합니다.

`AllowedOrigins`, `TrustedProxyCIDRs`, `MaxConnections`, `CapacityRetryAfter`, 인터셉터와 `PingInterval`은 커널이 관리합니다. 커널은 인증·출처·연결 수 검사를 통과한 요청만 플랫폼에 전달합니다. `WriteTimeout`은 각 쓰기 호출의 절대 시각으로 전달됩니다. 시간이 지정되지 않은 deadline은 제한 없음을 뜻합니다.

`Adapter.Upgrade`를 애플리케이션에서 직접 마운트하면 커널의 출처 검사와 인증을 거치지 않습니다. Spine 앱에서는 `WithWebSocketAdapter`로 선택하고 커널이 만든 업그레이드 경로를 사용합니다.

## 플랫폼 선택

```go
import (
    "github.com/NARUBROWN/spine"
    echoPlatform "github.com/NARUBROWN/spine-echo-platform"
    gorillaPlatform "github.com/NARUBROWN/spine-gorilla-platform"
)

app := spine.New(echoPlatform.Option(), gorillaPlatform.Option())
```

직접 구현한 factory는 `spine.WithWebSocketAdapter(factory)`로 주입합니다. WS 경로가 없으면 WS factory가 필요하지 않으며 생성하지도 않습니다. WS 경로가 있으면 HTTP 플랫폼의 `WebSocketUpgrade` 지원과 WS factory가 모두 필요합니다. factory가 없거나 계약 버전이 다르면 초기화 전에 `wsadapter.ErrInvalidAdapter`로 실패합니다.

Gorilla 구현은 독립 모듈 `github.com/NARUBROWN/spine-gorilla-platform`에서 제공합니다. 배포할 때는 이 계약과 `WithWebSocketAdapter`를 제공하는 Spine 버전을 먼저 고정하고, 플랫폼이 그 버전을 의존하도록 관리합니다. `spine/adapter/websocket` 경로는 같은 타입과 오류의 호환 별칭입니다. 이전 루트의 WebSocket 별칭 패키지는 제거했습니다.

검증 오류는 1008과 `invalid message`, 처리 오류는 1011과 `handler error`, 서버 정상 종료는 1000과 `server shutting down`으로 닫습니다. 선택한 validator와 이벤트 publisher는 WS 메시지 실행에도 같은 App 파이프라인으로 연결됩니다.

```sh
go test ./contract/websocket -count=1
bash scripts/check-module-boundaries.sh
go -C test test -race ./... -count=1
go -C test/platform-e2e test -race ./... -count=1
```
