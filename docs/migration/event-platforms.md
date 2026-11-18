# Migrate to event platforms

The development source moves Kafka and RabbitMQ implementations out of the core. Keep constructor registration and `app.Consumers().Register`; replace broker boot options with named event factories and migrate publishing to the [application DI Publisher](app-publisher.md). Use the exact core/platform source combination described by this branch, or a release that includes these APIs.

| Removed core API | Replacement |
| --- | --- |
| `boot.Options.Kafka`, `boot.KafkaOptions` | `spine-kafka-platform.Config` and `spine.WithEventAdapter` |
| `boot.Options.RabbitMQ`, `boot.RabbitMqOptions` | `spine-rabbitmq-platform.Config` and `spine.WithEventAdapter` |
| `Read` and `Write` pointers | Platform `Consume` and `Publish` declarations |
| Broker `ConsumerRetry` | `spine.WithEventConsumerRetry(boot.ConsumerRetryOptions{...})` |
| Settlement timeout | `spine.WithEventSettlementTimeout`, default 5 seconds |
| RabbitMQ failure policy, DLX, prefetch | RabbitMQ platform Config |
| RabbitMQ `PublisherRetry` | Removed; the failed message returns an error and a later call creates a new publisher |

Create a factory with `kafkaplatform.NewFactory(kafkaplatform.Config{Brokers: brokers, Consume: true, Publish: true, GroupID: "orders", PublishDestinations: destinations.Catalog{}})`, then create the App with `spine.New(echoplatform.Option(), spine.WithEventAdapter("kafka", factory))`. Handle the factory error before constructing the App. The import paths are `github.com/NARUBROWN/spine-kafka-platform` and `github.com/NARUBROWN/spine-echo-platform`. HTTP-less workers omit both the Echo option and `boot.Options.HTTP`.

RabbitMQ uses `rabbitmqplatform.NewFactory(rabbitmqplatform.Config{URL: address, Exchange: "events", Consume: true, Publish: true, PublishDestinations: destinations.Catalog{}})` and the same App option. Its import path is `github.com/NARUBROWN/spine-rabbitmq-platform`. TLS and native connection settings belong to the platform. Keep TLS defaults; plaintext requires explicit local-only opt-in.

Names must be non-empty and unique within an App. Roles come from immutable factory capabilities. Kafka TopicPrefix maps both publication and subscription names: with `prod.` configured, register logical `order.created` once, without duplicating the prefix. RabbitMQ defaults to FailureReject; select FailureRequeue explicitly and provision DLX exchange/queue bindings before enabling dead-letter options.

`Validate` checks names, versions, capabilities and common policies without constructing resources. `Prepare` snapshots registration and obtains publishers and ready readers without reading messages or starting handlers. On failure it rolls back acquired resources. `Run` follows the same preparation path and starts those readers without recreating them. A result returned by Prepare is caller-owned and exposes Handler/Close for externally served HTTP; it has no consumer Start API.

The kernel dispatches to the handler bound to the subscription passed to the factory. Producer event names and metadata do not select another handler. Consumers receive spine.EventCtx. Handler execution is followed by context cleanup and Ack on success, or Nack on failure. HTTP response processing and HTTP interceptors do not run for consumers. The application DI Publisher serializes each fields map once and waits for fanout to all Publish platforms. Multiple Consume platforms share the consumer registry; registration without a Consume platform remains dormant.

Reader creation and reconnection use common backoff. ConsumerRetry MaxAttempts zero means unlimited retries; invalid factory results fail immediately. Settlement uses a bounded context independent of run cancellation, defaulting to five seconds. Shutdown waits for active requests, consumer processing, settlements and reader cleanup before closing publishers, and preserves cleanup errors.

A failed publisher is retired after active calls finish. A subsequent publication creates a replacement with the same factory. The failed message is not automatically resent. Lost confirmations and consumer redelivery can produce duplicates; keep consumers idempotent. HTTP completion ordering is unchanged. A confirmed synchronous publish is not rolled back by later HTTP failure and is not atomic with a database commit. Durable outbox handling remains application work.

The destinations.Catalog in these fragments must implement PublishDestinations() []string and return the logical publish keys. See [cmd/reference](../examples/reference.md), the [Kafka example](../examples/kafka-publish.md), [security configuration](../security-configuration.md) and the preserved [Korean example](../ko/migration/event-platforms.md). The core no longer imports Kafka/RabbitMQ clients or concrete platforms. The legacy HTTP adapter import path remains compatible.

Run core tests and `scripts/check-module-boundaries.sh`. With `SPINE_TEST_KAFKA_BROKER` and/or `SPINE_TEST_RABBITMQ_URL` set, run `go -C test test -race . -run '^TestAppEventLive_' -count=1 -timeout=2m`. This checks HTTP publication, worker redelivery and result events with real brokers. Missing broker environment variables skip the corresponding live test. CLI static checks and the partial runtime probe are separate evidence.

## Cleanup error retention

Each event adapter keeps separate publisher and consumer cleanup histories. Defaults are the latest 32 failures for at most one hour from cleanup completion. Both limits apply. New failures evict the oldest entries at capacity, and timers release expired errors even while idle. All cleanup failures are logged; expired or evicted errors are omitted from the final shutdown result.

These options bound in-memory history; log storage retention is separate. Configure the two roles independently with `spine.WithEventPublisherErrorHistory` and `spine.WithEventConsumerErrorHistory`, each taking `boot.CleanupErrorHistoryOptions{MaxEntries: 64, MaxAge: 30*time.Minute}`. Zero fields select defaults; negative values fail preflight. `Close` and `Stop` freeze the retained error snapshot after cleanup completes. Later calls return that same shutdown result even after the retention window expires.

## Nil payloads and Kafka tombstones

Raw `[]byte` and named byte-slice handlers receive nil unchanged, preserving the distinction between a tombstone and a nonnil empty payload. A successful handler is acknowledged; handler failures still follow the NACK policy. DTO handlers that require a body reject nil through the existing decoding and validation failure path. Use a raw-byte handler for subscriptions that must process tombstones; DTO failures are not silently acknowledged or skipped.

[Korean original](../ko/migration/event-platforms.md).
