# Publish without an incoming request

The `cmd/reference/publish-only` example starts an explicit polling loop after application preparation. DI provides `publish.Publisher`; constructors only assemble dependencies.

```sh
SPINE_DEMO_RABBITMQ_URL=amqp://guest:guest@localhost:5672/ go -C cmd/reference run ./publish-only
```

This command targets a local test broker and explicitly enables insecure AMQP in the sample. The sample publishes `orders.created` to the `spine-publish-only` exchange.

A pending in-memory row has a stable ID and occurrence time. Every attempt uses those same values through `WithMessageID` and `WithOccurredAt`. Mark the row sent only after confirmation. Errors leave it pending. A per-call timeout bounds each send.

On SIGINT/SIGTERM, the shutdown hook cancels and joins the polling task before publishers close. The sample has one in-memory row; production outboxes need transactional persistence, worker leasing and idempotent consumers. A crash after confirmation and before recording success can still duplicate delivery.

See the [publisher migration](../migration/app-publisher.md) and [event contract](../reference/event-platform.md).
