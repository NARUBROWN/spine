# Event platform contract

`contract/event` separates broker I/O from application execution. Factories declare capabilities and create readers and native publishers. The application-facing DI publisher is a separate [publishing API](../migration/app-publisher.md).

## Messages and factory initialization

`Message` contains `EventName`, `Payload`, `OccurredAt` and string `Metadata`. `Subscription.Topic` is the logical subscription registered in the app. Factories map logical destinations to native topics, queues or routing keys.

Select the consumer handler from its trusted subscription. Producer-supplied names and metadata must not select another registered handler. Native metadata such as AMQP Type may be retained without changing routing.

Payload and metadata are borrowed reference data. Callers must not mutate them during an operation. Copy them before retaining them beyond the operation's documented lifetime. Raw byte consumers preserve nil payloads, including tombstones. DTO consumers can reject a missing body.

`NewReader(ctx, subscription)` must return a non-nil ready reader without invoking handlers or settlement. `NewPublisher(ctx)` prepares publishing resources, but does not prove that every destination exists or a future send will succeed. Reject invalid configuration with `ErrInvalidAdapter`; preserve cancellation identity with `errors.Is`.

Factories clean partially acquired resources on failure. On success the caller owns the resource. The factory context controls initialization only; canceling it after success must not close the resource. Every subsequent operation uses its own context.

## Delivery and settlement

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

A reader processes `Read -> handler -> Ack or Nack -> next Read` sequentially. Use independent readers for parallel processing. Returning a typed nil delivery, `(nil, nil)` or a delivery together with a read error violates the contract.

On success, the core acknowledges the delivery after handler execution and context cleanup. Handler errors and recovered panics lead to NACK. Consumers receive `spine.EventCtx`; HTTP interceptors and response handlers do not run for event deliveries.

Attempt exactly one settlement operation per delivery. A failed settlement does not permit an alternative ACK/NACK on the same delivery. Reject duplicate settlement with `ErrInvalidState`. Cancellation can leave the broker outcome uncertain.

`Nack` applies the configured broker policy. Kafka may return `ErrReaderInvalidated` to avoid committing past a failed offset. RabbitMQ may reject or requeue; dead-letter retention depends on actual topology. After read or settlement errors, close the reader before reconnecting. Preserve `errors.Is` identity.

## Native publisher

```go
type Publisher interface {
    Publish(context.Context, Message) error
    Close(context.Context) error
}
```

`Publish` is synchronous and supports concurrent calls. Do not modify or retain caller-owned buffers without copying. Success means the configured transport confirmation completed, such as Kafka required acknowledgements or RabbitMQ confirms and mandatory routing. It does not imply consumer processing, a database commit or duplicate-free delivery.

An error or cancellation can occur after broker acceptance. Retrying can duplicate delivery. Partial success across multiple publishers is not rolled back. The core retires a failed native publisher and recreates it on a later call; it does not automatically resend the failed event.

## Closing and resource ownership

Reader and publisher `Close` calls must be repeatable and safe concurrently. Stop admission, start cleanup once and wait within the caller's context. A timed-out wait does not abandon cleanup; a later call can wait again. Reject operations after closure with an invalid-state error. Cleanup errors preserve their causes.

Common consumer retry and settlement timeouts are selected through `WithEventAdapter` options. Retry limits apply to transport creation and reconnection, not business-handler attempts. See [platform migration](../migration/event-platforms.md), [security defaults](../security-configuration.md) and [broker tests](../testing/platforms.md).

[Korean original](../ko/reference/event-platform.md).
