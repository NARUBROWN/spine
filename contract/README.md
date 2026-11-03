# 공통 실행 계약

Spine과 외부 구현이 공유하는 공개 계약은 `contract/` 아래에서 관리합니다. 패키지는 표준 라이브러리만 사용하며 Spine 모듈과 함께 버전을 관리합니다.

| 패키지 | 책임 | 현재 연결 범위 |
| --- | --- | --- |
| [`contract/http`](http/README.md) | HTTP 입력·출력, factory와 서버 수명 | Spine HTTP 실행과 연결 |
| [`contract/event`](event/README.md) | 브로커 메시지, 구독, ACK/NACK, reader와 publisher | `WithEventAdapter`와 App 준비·실행·종료에 연결 |
| [`contract/websocket`](websocket/README.md) | WS 업그레이드·메시지 입출력·ping/close | `WithWebSocketAdapter`와 연결 수명에 연결 |
| [`contract/validator`](validator/README.md) | DTO 검증 인터페이스와 필드 오류 | 앱 옵션과 HTTP·WebSocket·컨슈머 DTO에 연결 |

구체 HTTP 프레임워크, 브로커 클라이언트와 검증 엔진은 외부 모듈이 소유합니다. `spine/adapter`, `spine/adapter/http`, `spine/adapter/event`, `spine/adapter/websocket` 경로는 같은 타입과 오류의 호환 별칭을 제공합니다. 이전 루트의 WebSocket 별칭 패키지는 `adapter/websocket`으로 이동했습니다.

```sh
go test ./contract/... -count=1
bash scripts/check-module-boundaries.sh
```

실제 구현체 조합은 [플랫폼 계약 E2E](../test/platform-e2e/README.md)에서 확인합니다. HTTP·폼·WebSocket 검증과 Kafka·RabbitMQ의 발행·컨슈머 검증·ACK/NACK를 함께 검사합니다.
