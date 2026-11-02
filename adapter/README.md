# 어댑터 계약

Spine의 공개 계약은 전송 종류별 패키지로 나눕니다. 계약은 표준 라이브러리만 사용하고 구체 구현은 `spine-XXX-platform` 모듈이 소유합니다.

| 계약 | 패키지 | 구현 모듈 |
| --- | --- | --- |
| HTTP | [`adapter/http`](http/README.md), package `httpadapter` | `spine-echo-platform` |
| 이벤트 | [`adapter/event`](event/README.md), package `eventadapter` | `spine-kafka-platform`, `spine-rabbitmq-platform` |

`platform`은 Spine 계약을 특정 기술로 구현하고 native 설정과 자원 수명을 관리하는 통합 모듈을 뜻합니다. HTTP 플랫폼은 HTTP 실행을, 브로커 플랫폼은 Reader와 Publisher를 제공합니다.

## 기존 HTTP 경로의 호환성

`github.com/NARUBROWN/spine/adapter`는 기존 HTTP 소비자를 위한 호환 패키지로 남습니다. 공개 타입은 `adapter/http`의 타입 별칭이고 계약 버전, 오류와 `ValidateFactory`도 같은 구현을 사용합니다. 기존 Echo 플랫폼과 앱은 import 경로를 바꾸지 않고 계속 사용할 수 있습니다.

새 HTTP 구현은 다음 경로를 사용합니다.

```go
import httpadapter "github.com/NARUBROWN/spine/adapter/http"
```

이벤트 구현은 다음 경로를 사용합니다.

```go
import eventadapter "github.com/NARUBROWN/spine/adapter/event"
```

이 패키지 재배치는 HTTP 계약 `spine.http-adapter/v2`의 메서드나 버전을 바꾸지 않습니다. 이벤트 계약은 `spine.event-adapter/v1`입니다. 이벤트 factory를 App에 등록하는 실행 연결은 별도 단계이며 기존 `boot.Options.Kafka`와 `boot.Options.RabbitMQ`의 동작은 유지됩니다.
