# Migrate to event platforms

The development source moves Kafka and RabbitMQ implementations out of the core. Keep `publish.Event`, constructor registration and `app.Consumers().Register`; replace broker boot options with named event factories. These APIs are not part of the existing Spine v0.6.0 release.

| Removed core API | Replacement |
| --- | --- |
| `boot.Options.Kafka`, `boot.KafkaOptions` | `spine-kafka-platform.Config` and `spine.WithEventAdapter` |
| `boot.Options.RabbitMQ`, `boot.RabbitMqOptions` | `spine-rabbitmq-platform.Config` and `spine.WithEventAdapter` |
| `Read` and `Write` pointers | Platform `Consume` and `Publish` declarations |
| Broker `ConsumerRetry` | `spine.WithEventConsumerRetry(boot.ConsumerRetryOptions{...})` |
| Settlement timeout | `spine.WithEventSettlementTimeout`, default 5 seconds |
| RabbitMQ failure policy, DLX, prefetch | RabbitMQ platform Config |
| RabbitMQ `PublisherRetry` | Removed; the failed message returns an error and a later call creates a new publisher |

Create a factory with `kafkaplatform.NewFactory(kafkaplatform.Config{Brokers: brokers, Consume: true, Publish: true, GroupID: "orders"})`, then create the App with `spine.New(echoplatform.Option(), spine.WithEventAdapter("kafka", factory))`. Handle the factory error before constructing the App. The import paths are `github.com/NARUBROWN/spine-kafka-platform` and `github.com/NARUBROWN/spine-echo-platform`. HTTP-less workers omit both the Echo option and `boot.Options.HTTP`.

RabbitMQ uses `rabbitmqplatform.NewFactory(rabbitmqplatform.Config{URL: address, Exchange: "events", Consume: true, Publish: true})` and the same App option. Its import path is `github.com/NARUBROWN/spine-rabbitmq-platform`. TLS and native connection settings belong to the platform. Keep TLS defaults; plaintext requires explicit local-only opt-in.

Names must be non-empty and unique within an App. Roles come from immutable factory capabilities. Kafka TopicPrefix maps both publication and subscription names: with `prod.` configured, register logical `order.created` once, without duplicating the prefix. RabbitMQ defaults to FailureReject; select FailureRequeue explicitly and provision DLX exchange/queue bindings before enabling dead-letter options.

`Validate` checks names, versions, capabilities and common policies without constructing resources. `Prepare` snapshots registration and obtains publishers and ready readers without reading messages or starting handlers. On failure it rolls back acquired resources. `Run` follows the same preparation path and starts those readers without recreating them. A result returned by Prepare is caller-owned and exposes Handler/Close for externally served HTTP; it has no consumer Start API.

The kernel dispatches to the handler bound to the subscription passed to the factory. Producer event names and metadata do not select another handler. Successful execution and post-processing lead to Ack; failures lead to Nack. The core serializes each domain event once before fanout to all Publish platforms. Multiple Consume platforms share the consumer registry; registration without a Consume platform remains dormant.

Reader creation and reconnection use common backoff. ConsumerRetry MaxAttempts zero means unlimited retries; invalid factory results fail immediately. Settlement uses a bounded context independent of run cancellation, defaulting to five seconds. Shutdown waits for active requests, consumer processing, settlements and reader cleanup before closing publishers, and preserves cleanup errors.

A failed publisher is retired after active calls finish. A subsequent publication creates a replacement with the same factory. The failed message is not automatically resent. Lost confirmations and consumer redelivery can produce duplicates; keep consumers idempotent. The post-hook/BeforeResponse order is unchanged, and this does not make DB commits and broker publication atomic. Durable outbox handling remains application work.

See `cmd/reference`, `examples/security-config` and the complete [Korean example](event-platforms.md). The core no longer imports Kafka/RabbitMQ clients or concrete platforms. The legacy HTTP adapter import path remains compatible.

Run core tests and `scripts/check-module-boundaries.sh`. With `SPINE_TEST_KAFKA_BROKER` and/or `SPINE_TEST_RABBITMQ_URL` set, run `go -C test test -race . -run '^TestAppEventLive_' -count=1 -timeout=2m`. This checks HTTP publication, worker redelivery and result events with real brokers. Missing broker environment variables skip the corresponding live test. CLI static checks and the partial runtime probe are separate evidence.
