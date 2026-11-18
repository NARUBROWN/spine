# 앱 수명의 DI Publisher로 이전

> 정리 전 한국어 원문입니다. 현재 소스의 계약과 실행 방법은 [영어 기본 문서](../../migration/app-publisher.md)를 기준으로 확인하세요.

이 개발 소스는 이벤트를 호출 시 전송하고 모든 플랫폼의 결과를 기다립니다. `publish.Event`, `publish.DomainEvents`, `publish.WithPublisher`, `publish.EventPublisher`, `publish.DomainEvent`와 실행별 EventBus 및 drain 경로를 제거했습니다. 이미 배포된 버전의 동작은 바뀌지 않습니다.

## 호출 변경

이전 코드는 실행별 큐에 적재했습니다. 반환 오류는 실제 전송 결과가 아니었습니다.

```go
err := publish.Event(ctx, "orders.created", publish.Fields{"id": id})
publish.DomainEvents(ctx, event)
```

서비스 생성자에서 `publish.Publisher`를 주입받고 메서드의 오류를 처리합니다. 별도 발행기 생성자는 등록하지 않습니다. 앱이 `WithEventAdapter`로 등록한 발행 플랫폼을 준비한 뒤 공유 Publisher를 제공합니다.

```go
type OrderService struct {
    publisher publish.Publisher
}

func (*OrderService) Constructor(publisher publish.Publisher) *OrderService {
    return &OrderService{publisher: publisher}
}

func (s *OrderService) Create(ctx context.Context, id string) error {
    if err := s.publisher.Event(ctx, "orders.created", publish.Fields{"id": id}); err != nil {
        return err
    }
    // 모든 플랫폼의 전송 결과가 성공한 뒤에 실행합니다.
    return nil
}
```

`app.Constructor(constructor.Service(&OrderService{}))`로 서비스를 등록합니다. HTTP 경계에서는 `sc.ExecutionContext().Context()`를 서비스에 넘깁니다. WebSocket과 소비 핸들러도 같은 서비스를 사용합니다. 핸들러를 직접 호출하는 테스트는 `publish.Publisher` 인터페이스를 구현한 대체 객체를 주입합니다. 프레임워크 조립 테스트는 `WithEventAdapter`에 테스트 Factory를 등록합니다.

```go
r.POST("/orders/:id", func(sc spine.Ctx, id input.Path[string]) (httpx.Response[string], error) {
    if err := service.Create(sc.ExecutionContext().Context(), *id); err != nil {
        return httpx.Response[string]{}, err
    }
    return httpx.Response[string]{Body: "sent"}, nil
})
```

## 발행 결과와 실행 순서

| 상황 | 결과 |
| --- | --- |
| 정상 호출 | 본문을 JSON으로 한 번 직렬화하고 등록된 발행 플랫폼을 병렬 호출한 뒤 모두 기다립니다. |
| 빈 목적지, 직렬화 실패, 이미 취소된 context | 전송 전에 오류를 반환합니다. `nil` Fields는 `{}`가 됩니다. |
| 발행 플랫폼 없음 | `publish.ErrUnavailable`을 반환합니다. 처리하지 않은 HTTP 오류의 기본 상태는 503입니다. |
| 목적지 미등록, 연결 또는 전송 실패 | 플랫폼 오류를 감싸서 반환합니다. `errors.Is`와 `errors.As`로 원인을 확인할 수 있습니다. |
| 다중 플랫폼의 부분 실패 | 성공한 전송은 유지합니다. 모든 플랫폼을 기다리고 실패를 등록 순서로 `errors.Join`합니다. 오류에는 플랫폼 이름이 들어갑니다. |
| 발행 성공 후 핸들러 또는 응답 직렬화 실패 | 이미 성공한 전송은 유지하고 일반 오류 처리와 응답 처리를 진행합니다. |
| 종료 후 발행 | `contract/event.ErrInvalidState`로 비교할 수 있는 오류를 반환합니다. |

성공은 플랫폼의 전송 확인 정책이 완료됐다는 뜻입니다. RabbitMQ는 mandatory 라우팅과 publisher confirm을 확인합니다. 소비자의 업무 처리 완료까지 기다리지는 않습니다. 인터셉터의 발행도 호출 즉시 수행하며 그 위치에서 오류를 처리해야 합니다. 다른 인터셉터, 일반 후처리 훅과 응답 처리 순서는 유지됩니다.

발행한 이벤트와 이후 DB 커밋은 원자적이지 않습니다. 응답 준비 성공에 따라 발행을 억제하던 동작도 제거했습니다. DB 변경과 이벤트의 내구성을 함께 보장해야 하면 트랜잭션에 Outbox 행을 기록하고 독립 폴링 서비스에서 발행합니다.

## Outbox 식별자와 재시도

```go
err := publisher.Event(callCtx, row.Destination, publish.Fields{
    "id": row.ID,
    "order_id": row.OrderID,
}, publish.WithMessageID(row.ID),
   publish.WithOccurredAt(row.CreatedAt),
   publish.WithMetadata(map[string]string{"trace_id": row.TraceID}))
if err != nil {
    return err // 발행 완료를 기록하지 않습니다.
}
return repository.MarkPublished(callCtx, row.ID)
```

| 옵션 | 의미 |
| --- | --- |
| `WithMessageID(id)` | `Message.Metadata["message_id"]`에 안정적인 식별자를 전달합니다. 같은 키의 메타데이터보다 우선합니다. 식별자를 자동 생성하지 않습니다. |
| `WithOccurredAt(at)` | 원래 이벤트 시각을 보존합니다. 생략하거나 0이면 호출 시각을 사용합니다. |
| `WithMetadata(values)` | 메타데이터를 복사합니다. 옵션을 여러 호출에서 재사용할 수 있습니다. |

RabbitMQ에서 식별자는 AMQP 헤더로 전달됩니다. AMQP의 native `MessageId` 필드를 설정한다는 의미는 아닙니다. 소비자가 본문으로 중복을 판별한다면 식별자를 Fields에도 포함합니다.

실패한 메시지를 자동으로 다시 보내지 않습니다. 전송 오류나 취소 뒤에도 브로커가 이미 메시지를 받았을 수 있습니다. 다중 플랫폼 일부 성공 뒤 전체 호출을 재시도해도 중복이 생길 수 있습니다. 같은 식별자와 원래 시각으로 재시도하고 소비자는 멱등성을 보장해야 합니다.

## 폴링 서비스와 종료

HTTP나 소비 핸들러가 없는 앱도 `Prepare`가 끝나면 주입된 Publisher를 사용할 수 있습니다. 최초 요청은 필요하지 않습니다. DI 생성자는 서비스만 조립하고 `Run(ctx)`은 명시적으로 시작합니다. [publish-only 실행 예제](../../../cmd/reference/publish-only/main.go)는 이 순서를 보여줍니다. 예제의 대기 행은 메모리에 있으므로 운영 환경에서는 영속 저장소가 필요합니다.

종료는 전송 실행 정리, 등록 순서의 `WithShutdownHook`, 새 발행 차단, 플랫폼 발행기 정리 순서로 진행합니다. 종료 훅에서 폴링 context를 취소하고 작업 완료를 기다립니다. 훅 실행 중에는 발행기를 사용할 수 있습니다. 새 발행 차단 전에 들어온 호출의 반환도 기다립니다. 이미 플랫폼에 들어간 전송은 호출 context에 따라 완료되며, 아직 직렬화 중이거나 재연결을 기다리던 호출은 종료 오류 또는 취소 오류를 받을 수 있습니다. 재연결 중인 Factory 생성에는 종료 취소를 전달합니다.

호출 context의 취소는 해당 작업에 적용합니다. 공유 Publisher에 `Close`를 호출하지 않습니다. 실제 전송 자원이 실패하거나 무효화되면 진행 중 호출이 끝난 뒤 정리하고 다음 명시적 호출에서 재생성합니다. 앱의 `Close(ctx)`는 호출자의 완료 대기만 제한합니다. 시간이 초과돼도 정리는 계속되며 새 context로 다시 기다릴 수 있습니다. 진행 중 전송에는 호출자가 지정한 취소와 제한 시간이 계속 적용됩니다.

## 별도 저장소의 후속 변경

- `spine-cli`: 새 계약을 인식하는 CLI 변경을 함께 사용합니다. 정적 DI 검사는 실제 코어 선언으로 앱 제공 Publisher를 확인하고 별칭 및 구조적 인터페이스를 런타임 규칙에 맞게 해결합니다. 직접 제공자를 등록하면 `EVENT_PUBLISHER_RESERVED`로 거부합니다. 수명 테스트와 브로커 fixture도 DI 메서드 호출로 이전했으며 이전 공개 코어의 테스트는 해당 API를 선택합니다. 이전 CLI에서는 `COMPONENT_DEPENDENCIES_INVALID` 오류가 발생할 수 있습니다.
- `spine-user-demo`, `spine-simple-msa-demo`, `spine-simple-chat-demo` 및 `spine-site`: 이전 발행 호출과 큐 기반 설명이 있다면 새 계약으로 이전해야 합니다.
- Kafka, RabbitMQ 및 NATS 플랫폼: 직렬화된 `contract/event.Publisher.Publish` 계약과 목적지 선언은 유지됩니다. 코어 모듈 버전 갱신 후 실제 브로커 테스트가 필요합니다.

## 검증

코어 검사는 `go run ./scripts/test test -race ./... -count=1`로 실행합니다. `test/app_publisher_test.go`는 DI 공유, HTTP 및 WebSocket 발행, 독립 실행, 발행 이후 실패와 종료 순서를 검사합니다. 실제 RabbitMQ 검사는 격리된 테스트 브로커의 URL을 지정합니다.

```sh
SPINE_TEST_RABBITMQ_URL=amqp://guest:guest@127.0.0.1:5672/ \
  go -C test test -race -v . -run '^TestAppPublisherLive_' -count=1
```

실제 브로커 검사는 confirm 성공, unroutable 오류, 같은 Outbox 식별자의 재시도, 테스트 전용 TCP 연결 단절 후 재연결과 목적지 검증을 수행합니다. 테스트 대체 구현의 성공과 실제 브로커 결과를 구분합니다.
