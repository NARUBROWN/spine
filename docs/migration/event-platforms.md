# 이벤트 플랫폼으로 전환하기

현재 개발 소스는 Kafka·RabbitMQ 구현을 외부 플랫폼으로 분리합니다. `publish.Event`와 `app.Consumers().Register`는 유지하고, 브로커 설정과 App 연결을 바꿉니다. 이 안내는 개발 브랜치의 API이며 기존 Spine v0.6.0 릴리스에 소급 적용되지 않습니다.

## 제거된 설정

| 기존 코어 설정 | 새 위치 |
| --- | --- |
| `boot.Options.Kafka`, `boot.KafkaOptions` | `spine-kafka-platform.Config`와 `spine.WithEventAdapter` |
| `boot.Options.RabbitMQ`, `boot.RabbitMqOptions` | `spine-rabbitmq-platform.Config`와 `spine.WithEventAdapter` |
| `Read`, `Write` 포인터 | 플랫폼 Config의 `Consume`, `Publish` 역할 선언 |
| 브로커별 `ConsumerRetry` | `spine.WithEventConsumerRetry(boot.ConsumerRetryOptions{...})` |
| 전송 방식과 무관한 ACK/NACK 제한 | `spine.WithEventSettlementTimeout(duration)`, 기본 5초 |
| RabbitMQ `FailurePolicy`, `DeadLetter`, prefetch | RabbitMQ 플랫폼 Config |
| RabbitMQ `PublisherRetry` | 제거. 현재 메시지의 실패는 반환하고 다음 발행 때 새 publisher를 생성 |

코어는 브로커 클라이언트와 플랫폼 모듈을 import하지 않습니다. `boot.ConsumerRetryOptions`는 공통 재연결 정책으로 유지합니다. HTTP 계약의 기존 `spine/adapter` 호환 경로도 유지합니다.

## Kafka App 연결

애플리케이션 모듈에 `github.com/NARUBROWN/spine-kafka-platform`을 추가합니다. 현재 코어와 호환되는 버전은 플랫폼 go.mod의 Spine 의존성을 확인합니다. 저장소 예제의 로컬 replace는 소스 개발용입니다.

```go
import (
    "time"

    "github.com/NARUBROWN/spine"
    echoplatform "github.com/NARUBROWN/spine-echo-platform"
    kafkaplatform "github.com/NARUBROWN/spine-kafka-platform"
    "github.com/NARUBROWN/spine/pkg/boot"
)

func buildApp() (spine.App, boot.Options, error) {
    factory, err := kafkaplatform.NewFactory(kafkaplatform.Config{
        Brokers: []string{"kafka.example.com:9093"},
        Consume: true,
        Publish: true,
        GroupID: "order-service",
        TopicPrefix: "prod.",
    })
    if err != nil { return nil, boot.Options{}, err }
    app := spine.New(echoplatform.Option(), spine.WithEventAdapter("kafka", factory,
        spine.WithEventConsumerRetry(boot.ConsumerRetryOptions{MaxAttempts: 20}),
        spine.WithEventSettlementTimeout(5*time.Second),
    ))
    // 기존 생성자·HTTP 경로·Consumers 등록을 이어서 사용합니다.
    return app, boot.Options{HTTP: &boot.HTTPOptions{}}, nil
}
```

HTTP 없는 워커는 Echo 옵션과 `boot.Options.HTTP`를 생략합니다. factory의 역할은 불변 `Capabilities`에서 읽으므로 App 쪽에서 Read/Write를 다시 선언하지 않습니다. 플랫폼 이름은 비어 있지 않고 App 안에서 고유해야 합니다.

Kafka의 TopicPrefix는 발행 이름과 논리 구독 키 양쪽에 적용됩니다. 위 설정에서는 `app.Consumers().Register("order.created", handler)`가 `prod.order.created`를 구독합니다. 접두사를 등록 키에 다시 넣지 않습니다. TLS·SASL·native Dialer·Transport는 Kafka 플랫폼이 소유합니다.

## RabbitMQ App 연결

`rabbitmqplatform.NewFactory(rabbitmqplatform.Config{URL: address, Exchange: "events", Consume: true, Publish: true})`로 만든 factory를 `spine.WithEventAdapter("rabbitmq", factory)`에 넘깁니다. import 경로는 `github.com/NARUBROWN/spine-rabbitmq-platform`입니다.

실패 기본값은 `rabbitmqplatform.FailureReject`입니다. 재큐잉을 원하면 `FailureRequeue`, dead-letter 구성을 원하면 `DeadLetter`를 명시합니다. DLX와 대상 큐의 binding은 먼저 준비해야 합니다. `OperationTimeout`은 플랫폼의 송신·확인 대기 한계이고, 코어의 SettlementTimeout은 전달 확인 호출 전체의 한계입니다.

현재 RabbitMQ 플랫폼은 실패한 메시지를 자동 재발행하지 않습니다. App은 실패한 publisher를 진행 호출이 끝난 뒤 정리하고 다음 발행 때 factory로 새 인스턴스를 만듭니다. 확인 유실 뒤 재시도나 consumer 재전달은 중복을 만들 수 있으므로 멱등 처리를 유지합니다.

## 실행·종료 순서

1. `Validate`는 이름·계약 버전·기능과 공통 정책을 검사합니다. 네트워크와 생성 메서드를 실행하지 않습니다.
2. `Prepare`는 등록을 복사하고 publisher 및 구독별 reader를 준비합니다. reader를 읽거나 핸들러를 시작하지 않습니다. 실패하면 획득한 자원을 정리합니다.
3. `Run`은 같은 준비 경로를 거친 뒤 그 실행에서 준비한 reader를 사용합니다. reader를 다시 생성해 검증을 반복하지 않습니다.
4. 코어는 factory에 전달한 구독 키로 핸들러를 선택합니다. 수신 EventName이나 생산자 메타데이터는 재라우팅에 쓰지 않습니다.
5. 핸들러와 후처리가 성공하면 Ack, 실패하면 Nack을 호출합니다. 도메인 이벤트는 한 번 직렬화한 뒤 모든 Publish 역할의 플랫폼에 보냅니다. 실패한 reader는 backoff 후 재생성합니다.
6. 종료 시 진행 중인 HTTP 요청과 consumer 실행·ACK/NACK·reader 정리를 끝낸 뒤 publisher를 닫습니다. 확인 context는 실행 취소와 독립적으로 제한 시간까지 기다립니다. 정리 오류도 보존합니다.

여러 Consume 플랫폼을 등록하면 같은 consumer 등록 목록을 사용합니다. consumer 등록만 있고 Consume 플랫폼이 없으면 소비는 시작하지 않습니다. `MaxAttempts: 0`은 소비 재연결의 무제한 재시도이며, 잘못된 factory 결과는 재시도하지 않습니다.

`Prepare`가 반환한 결과는 호출자가 Close합니다. 그 결과에 consumer Start API는 없으며, 외부 HTTP 서버에 Handler를 연결하는 용도입니다. `Run`/`RunContext`가 관리 실행을 시작합니다.

이벤트 발행과 DB 커밋은 원자적이지 않습니다. 기존 post-hook과 BeforeResponse 순서는 유지하며, durable outbox와 중복 제거는 별도로 구성해야 합니다.

## 검증

`cmd/reference`와 `examples/security-config`는 새 App 구성을 사용합니다. `test/app_event_live_test.go`는 실제 HTTP 요청에서 이벤트를 발행하고, 실패한 전달을 재처리하는 워커가 결과 이벤트를 보내는 흐름을 두 브로커에서 검사합니다.

```sh
go run ./scripts/test test -race ./... -count=1
bash scripts/check-module-boundaries.sh
# 테스트용 브로커 주소를 설정한 뒤 실행합니다.
go -C test test -race . -run '^TestAppEventLive_' -count=1 -timeout=2m
```

브로커 환경변수는 `SPINE_TEST_KAFKA_BROKER`, `SPINE_TEST_RABBITMQ_URL`입니다. 설정하지 않은 브로커의 테스트는 건너뜁니다. 최소 HTTP/WS 데모의 CLI 정적 검사와 부분 runtime probe는 브로커 E2E를 대신하지 않습니다.

## 정리 실패 이력

각 이벤트 어댑터의 publisher와 consumer는 실제 자원 정리 실패를 별도로 보관합니다. 기본값은 역할별 최근 32건, 정리 실패 시점부터 최대 1시간입니다. 건수와 기간 중 먼저 도달한 한도를 적용합니다. 새 실패로 건수를 초과하면 가장 오래된 오류를 제외하고, 기간이 지난 오류는 유휴 상태에서도 타이머로 제거합니다. 전체 실패는 로그에 남기며 만료되거나 제외된 오류는 최종 종료 오류에 포함하지 않습니다.

```go
spine.WithEventAdapter("events", factory,
    spine.WithEventPublisherErrorHistory(boot.CleanupErrorHistoryOptions{
        MaxEntries: 64,
        MaxAge:     30 * time.Minute,
    }),
    spine.WithEventConsumerErrorHistory(boot.CleanupErrorHistoryOptions{
        MaxEntries: 16,
        MaxAge:     10 * time.Minute,
    }),
)
```

이 옵션은 메모리 이력만 제한하며 로그 저장소의 보관 정책은 별도입니다. 각 필드의 0은 기본값을 사용하고 음수는 앱 사전 검사에서 거부합니다. `Close`와 `Stop`은 정리를 마친 시점에 보관 중인 오류만 합쳐 종료 결과를 고정합니다. 이후 같은 종료를 기다리는 호출에는 같은 결과를 반환합니다. 이 결과는 보관 기간이 지나도 바뀌지 않으며, 이미 반환한 오류를 취소하지 않습니다.

## nil 본문과 Kafka tombstone

raw `[]byte` 및 이름 있는 byte slice 핸들러는 nil을 그대로 받습니다. nil과 길이 0인 nonnil slice를 합치지 않으므로 Kafka tombstone을 구분할 수 있습니다. nil을 받은 핸들러가 성공하면 ACK하고, 핸들러가 실패하면 기존 NACK 정책을 적용합니다.

본문이 필수인 DTO 핸들러는 nil 본문을 거부하고 기존 역직렬화 및 검증 실패 경로를 사용합니다. tombstone이 들어오는 구독은 raw byte 핸들러에서 명시적으로 처리하세요. DTO 핸들러의 실패를 무조건 ACK하거나 건너뛰지 않습니다.
