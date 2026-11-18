# Migrate to the application DI Publisher

The current source provides one `publish.Publisher` per application. Inject it into services and use it from HTTP, WS, consumers or background work. The former package-level `publish.Event`, execution-local EventBus and drain path are removed.

```go
type OrderService struct { publisher publish.Publisher }

func (*OrderService) Constructor(publisher publish.Publisher) *OrderService {
    return &OrderService{publisher: publisher}
}

func (s *OrderService) Create(ctx context.Context, id int64) error {
    return s.publisher.Event(ctx, "orders.created", publish.Fields{"order_id": id})
}
```

Register the service with `constructor.Service`. The application reserves the exact Publisher interface; do not register your own provider. Register native platforms with `spine.WithEventAdapter`. Without an enabled publishing platform, calls return `publish.ErrUnavailable`.

## Context and return value

| Caller | Context passed to Publisher.Event |
| --- | --- |
| HTTP callback | `ctx.ExecutionContext().Context()` from `spine.Ctx`. |
| WS callback | `spine.WebSocketCtx` or its standard context. |
| Event consumer | `spine.EventCtx`. |
| Background worker | A context owned and canceled by that worker's owner. |

`Event(context.Context, string, Fields, ...Option) error` serializes the fields once, fans out concurrently to enabled publishers and waits for every operation. It preserves caller cancellation and joins platform errors. Handle the returned error immediately. A later handler error or response-serialization failure cannot roll back an already confirmed send.

## Stable message identity

Use `WithMessageID`, `WithOccurredAt` and `WithMetadata` when retrying a stored event. Preserve the same ID and occurrence time across attempts. Message IDs are carried in `Message.Metadata["message_id"]`; this does not imply use of a broker's native message-ID field. Zero occurrence time uses the current time. Metadata options snapshot their input.

Partial platform success, timeout or cancellation can leave delivery uncertain. The core does not automatically resend a failed message. Native connection failures retire the affected native publisher and a later application call can recreate it. A canceled caller does not retire a healthy shared connection solely because its own context ended.

## Background work and shutdown

Constructors assemble dependencies. Start workers explicitly after preparation. Register a shutdown hook that cancels and joins workers while Publisher is still available. The application then stops publisher admission, closes native platforms and waits for admitted calls.

For database-plus-event atomicity, insert a durable outbox row in the same transaction as business data. Publish it from a separate relay, mark it sent after confirmation and deduplicate consumers. The [publish-only sample](../examples/publish-only.md) uses a single in-memory row and is not a production durable outbox.

See [event contracts](../reference/event-platform.md), [reference application](../examples/reference.md) and [current source migration](v0.7.6.md).

[Korean original](../ko/migration/app-publisher.md).
