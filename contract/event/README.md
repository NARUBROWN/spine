# 이벤트 어댑터 계약

`github.com/NARUBROWN/spine/contract/event`는 브로커 구현자가 사용할 공개 전송 계약입니다. `Reader`로 메시지를 받아 커널에 전달하고, `Publisher`로 직렬화된 이벤트를 발행합니다. 계약 버전은 `spine.event-adapter/v1`이며 Spine 모듈과 함께 관리합니다.

앱은 `spine.WithEventAdapter(name, factory)`로 구현을 등록합니다. App은 역할 선언을 검사하고, 준비 과정에서 publisher와 ready reader를 획득한 뒤 실행·결과 확인·종료에 연결합니다. 기존 `boot.Options.Kafka`·`RabbitMQ`와 코어의 구체 브로커 구현은 제거했습니다. [App 연결과 마이그레이션](../../docs/migration/event-platforms.md)을 참고하세요.

## 책임과 의존성

계약을 런타임에 연결할 때의 책임은 다음과 같습니다.

| 위치 | 책임 |
| --- | --- |
| Spine 커널 | 도메인 이벤트 수집, 직렬화, DI, 구독과 핸들러 연결, 실행 결과 판단, dispatch와 재연결 조정 |
| `eventcontract` | 메시지, 구독, 읽기·발행, 전달 확인, factory와 자원 수명 계약 |
| 브로커 구현 | 연결·인증과 native 설정, 구독 키 매핑, 실제 송수신, offset commit·ACK/NACK·발행 확인 |

계약 패키지는 표준 라이브러리만 import합니다. `core.ExecutionContext`, `DomainEvent`, handler metadata나 native 클라이언트 타입을 요구하지 않습니다. HTTP 계약과는 별도 패키지이며 서로 의존하지 않습니다. 브로커별 typed 옵션과 검증은 외부 구현이 소유합니다.

## Factory와 기능 선언

```go
type Factory interface {
    Capabilities() Capabilities
    NewReader(context.Context, Subscription) (Reader, error)
    NewPublisher(context.Context) (Publisher, error)
}
```

`Capabilities`에는 `ContractVersion`, `Consume`, `Publish`가 있습니다. 읽기와 발행 중 하나 이상을 지원해야 하며 두 역할은 독립적입니다. `Requirements`는 호출자가 필요한 역할을 지정합니다.

```go
func checkConsumer(factory eventcontract.Factory) error {
    return eventcontract.ValidateFactory(factory, eventcontract.Requirements{
        Consume: true,
    })
}
```

이 예제는 `eventcontract "github.com/NARUBROWN/spine/contract/event"`를 import한 코드에서 사용합니다. `ValidateFactory`는 nil과 typed nil, 정확한 계약 버전, 지원 역할을 확인합니다. 유효한 factory의 `Capabilities`를 한 번 조회하며 `NewReader`와 `NewPublisher`를 호출하지 않습니다. 연결 상태, 생성 결과와 실제 전달 보장은 검사하지 않습니다.

`Capabilities`는 네트워크 연결이나 native 훅을 실행하지 않는 불변 선언이어야 합니다. 지원하지 않는 생성 메서드를 직접 호출하면 `ErrUnsupportedCapability`로 감싼 오류와 nil 결과를 반환합니다. factory의 메서드는 동시에 호출할 수 있어야 하며 생성 호출마다 독립된 수명의 자원을 반환합니다.

## 메시지와 구독

| 타입·필드 | 의미와 제약 |
| --- | --- |
| `Message.EventName string` | 발행 이벤트의 비어 있지 않은 논리 이름. 수신 시 알 수 없으면 구독 키를 사용합니다. |
| `Message.Payload []byte` | 직렬화된 본문. nil과 빈 본문도 허용하며 해석은 커널과 애플리케이션이 맡습니다. |
| `Message.OccurredAt time.Time` | 이벤트 발생 시각. 수신 시 알 수 없으면 시각을 지정하지 않습니다. |
| `Message.Metadata map[string]string` | 추적 정보 등 문자열 메타데이터. nil을 허용하며 native 매핑과 제한은 구현이 명시합니다. |
| `Subscription.Topic string` | 커널에 등록된 비어 있지 않은 논리 구독 키. native topic·queue·routing key와의 매핑은 factory 설정으로 결정합니다. |

커널은 `NewReader`에 넘긴 구독에 연결된 핸들러를 실행합니다. 생산자가 지정한 이벤트 이름이나 메타데이터를 사용해 다른 등록 핸들러로 재라우팅하지 않습니다. 예를 들어 RabbitMQ의 AMQP Type은 메시지 메타데이터로 보존할 수 있지만 커널의 핸들러 선택을 바꾸지 않습니다.

커널은 발행 전에 도메인 객체를 직렬화합니다. 어댑터는 받은 바이트를 해석하거나 다시 직렬화하지 않습니다. JSON은 사용할 수 있는 본문 형식 중 하나이며 계약 자체는 특정 형식을 강제하지 않습니다. 브로커별 본문 크기나 메타데이터 제약에 맞지 않는 입력은 전송 전에 오류로 반환해야 합니다.

`Payload`와 `Metadata`는 참조 데이터입니다. 수신 메시지의 참조는 해당 전달의 `Ack`/`Nack`이 끝나거나 `Reader.Close`가 시작되기 전까지 유효합니다. 커널은 이 데이터를 변경하지 않으며 더 오래 보관하려면 복사합니다. `Delivery.Message()`는 그 수명 안에서 같은 메시지 뷰를 반환합니다.

## 생성과 readiness

모든 context 인자는 nil이 아니어야 합니다. 생성 메서드는 잘못된 구독이나 구성에 `ErrInvalidAdapter`로 감싼 오류를 반환하고, 취소·시간 초과 오류는 `errors.Is`로 원래 context 오류를 확인할 수 있게 보존합니다.

| 호출 | 성공 조건과 소유권 |
| --- | --- |
| `NewReader(ctx, subscription)` | 연결과 구독 준비를 실제로 확인한 nil이 아닌 Reader를 반환합니다. 핸들러 실행과 ACK/NACK은 시작하지 않습니다. |
| `NewPublisher(ctx)` | 발행에 필요한 자원을 준비한 nil이 아닌 Publisher를 반환합니다. 각 발행 대상의 존재나 실제 전달 성공까지 보장하지 않습니다. |

Reader를 지연 연결하는 클라이언트는 생성자 호출만으로 readiness를 충족하지 못합니다. 예를 들어 Kafka 구현은 실제 프로토콜 요청 등으로 구독을 읽을 준비가 됐는지 확인해야 합니다. 이 준비가 권한·토폴로지의 어느 범위까지 확인하는지는 구현이 문서화합니다.

생성 중 취소되거나 실패하면 factory가 획득한 자원을 정리하고 `(nil, err)`를 반환합니다. 성공하면 호출자가 소유권을 받아 `Close`해야 합니다. 생성 context는 초기화에만 사용합니다. 성공 후 생성 context를 취소해도 반환한 자원의 수명은 끝나지 않으며, 이후 작업은 각각의 context를 따릅니다.

## 수신과 전달 확인

```go
type Delivery interface {
    Message() Message
    Ack(context.Context) error
    Nack(context.Context) error
}

type Reader interface {
    Read(context.Context) (Delivery, error)
    Close(context.Context) error
}
```

한 Reader에서는 `Read → 핸들러와 후처리 → Ack 또는 Nack → 다음 Read` 순서로 실행합니다. 하나의 전달이 확인되기 전에 다음 메시지를 읽지 않습니다. 병렬 처리는 독립된 Reader로 구성하며, 브로커가 보장하는 순서의 범위는 구현이 명시합니다.

- `Read`는 nil이 아닌 전달 또는 오류를 반환합니다. typed nil 전달과 `(nil, nil)`은 계약 위반입니다. 오류를 반환할 때는 전달을 함께 반환하지 않습니다.
- 핸들러와 커널 후처리가 모두 성공하면 `Ack`를 호출합니다. 실행에 실패하거나 패닉을 오류로 변환했다면 `Nack`을 호출합니다.
- 전달마다 `Ack`와 `Nack` 중 하나만 한 번 호출합니다. 오류가 반환돼도 같은 전달에 다른 확인을 시도하지 않습니다. 중복 확인 호출은 `ErrInvalidState`로 감싼 오류를 반환하며 브로커 작업을 반복하지 않습니다.
- `Ack`와 `Nack`은 context 취소에 응답해야 합니다. 취소돼도 브로커가 이미 확인을 받았을 수 있으므로 결과가 확정되지 않을 수 있습니다.

`Nack`은 factory에 명시된 실패 정책을 실행합니다. Kafka는 실패 offset을 넘어서 커밋하지 않도록 `ErrReaderInvalidated`를 반환할 수 있습니다. RabbitMQ는 설정에 따라 reject 또는 requeue를 수행할 수 있습니다. reject된 메시지의 보존 여부는 실제 dead-letter 토폴로지에 달려 있습니다. 공통 계약은 이 정책들을 같은 전달 보장으로 취급하지 않습니다.

`Read` 오류 또는 `Ack`/`Nack` 오류가 발생하면 커널은 같은 Reader를 더 읽지 않고 닫습니다. 계속 실행할 때는 재연결 정책에 따라 새 Reader를 생성합니다. 호출 context가 취소됐다면 재연결을 시작하지 않습니다. `ErrReaderInvalidated`는 의도적으로 세션을 교체해야 한다는 신호이며 `errors.Is`로 검사합니다.

## 발행과 확인

```go
type Publisher interface {
    Publish(context.Context, Message) error
    Close(context.Context) error
}
```

`Publish`는 동기 호출이며 같은 Publisher에 동시에 호출할 수 있습니다. 입력 메시지를 변경하지 않아야 하고, 반환 뒤에도 본문이나 메타데이터를 보관하려면 어댑터가 복사해야 합니다. 호출자도 발행 중 참조 데이터를 변경하지 않습니다.

nil 반환은 구현과 설정에 명시된 전송 확인 정책이 끝났다는 뜻입니다. Kafka의 required acknowledgements나 RabbitMQ의 publisher confirm·mandatory routing처럼 정책과 한계는 구현 문서에 명시합니다. nil 반환만으로 소비 완료, DB 커밋, 모든 브로커의 내구성이나 중복 없는 전달을 추론하지 않습니다.

오류나 취소가 반환돼도 브로커가 이미 메시지를 받았을 수 있습니다. 재발행은 중복을 만들 수 있으므로 멱등 처리와 재시도 정책은 별도로 설계해야 합니다. 여러 publisher 중 일부만 성공한 경우도 이 계약이 롤백하지 않습니다.

## 종료와 자원 수명

`Reader.Close`와 `Publisher.Close`는 여러 번 또는 동시에 호출할 수 있습니다. 첫 호출은 새 작업을 차단하고 정리를 한 번 시작합니다. 대기 중인 `Read`를 해제하고 이미 진행 중인 전달 확인·발행이 끝난 뒤 native 연결을 닫습니다. 종료가 시작된 뒤의 새 `Read`, `Publish`, `Ack`와 `Nack`은 `ErrInvalidState`로 감싼 오류를 반환합니다.

정상 종료에서는 커널이 처리 중인 전달의 확인을 먼저 끝냅니다. `Close` 자체는 미확인 전달을 성공으로 ACK하지 않습니다. 미확인 전달의 재전달 여부는 브로커 정책을 따릅니다.

`Close(ctx)`의 context는 호출자의 완료 대기만 제한합니다. 정리는 호출 context와 독립적으로 계속되며, 대기 시간이 끝나면 context 오류를 반환합니다. 이후 새 context로 `Close`를 호출해 같은 정리의 완료를 기다릴 수 있습니다. 완료 후 반복 호출은 보존된 같은 정리 결과를 반환해야 합니다.

## 오류

| 오류 | 의미 |
| --- | --- |
| `ErrInvalidAdapter` | nil 객체, 잘못된 구성·구독, 계약 버전 불일치 또는 전송 역할 없는 factory |
| `ErrUnsupportedCapability` | 요구한 읽기 또는 발행 역할을 구현하지 않은 factory |
| `ErrInvalidState` | 종료된 자원에 새 작업 요청 또는 같은 전달의 중복 확인 |
| `ErrReaderInvalidated` | 다음 읽기 전에 Reader를 닫고 다시 생성해야 하는 상태 |

어댑터는 `%w` 등으로 오류 원인을 보존합니다. 호출자는 문자열 대신 `errors.Is`를 사용합니다. 이 오류들은 DB와 브로커의 원자적 처리, outbox, exactly-once나 dead-letter 보존을 제공하지 않습니다.

## 검증

```sh
go run ./scripts/test test ./contract/event -count=1
bash scripts/check-module-boundaries.sh
```

테스트는 공개 타입만으로 외부 구현의 메서드 시그니처를 만족할 수 있는지, 그리고 부수 효과 없이 factory 선언을 검증하는지 확인합니다. 모듈 경계 검사는 HTTP와 이벤트 계약 각각의 테스트 의존성까지 표준 라이브러리로 제한합니다. 실제 브로커 동작과 수명 규칙은 각 어댑터의 구현 및 통합 테스트로 검증해야 합니다.

관련 계약: [HTTP 어댑터](../http/README.md), [이벤트 계약 소스](contract.go).
