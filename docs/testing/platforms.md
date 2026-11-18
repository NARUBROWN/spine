# Verify platform and broker combinations

Use `test/platform-e2e` and `test/nats-e2e` to verify concrete HTTP, WS, broker and validation modules against a selected core. Local source replacements and public installation are separate test modes.

```sh
go -C test/platform-e2e test -race ./... -count=1
go -C test/nats-e2e test -race ./... -count=1
```

Configure the broker endpoints expected by the tests. Common application live tests use `SPINE_TEST_KAFKA_BROKER`, `SPINE_TEST_RABBITMQ_URL` and `SPINE_TEST_NATS_URL`; inspect the selected test's environment requirements. Missing endpoints skip opt-in cases rather than proving broker behavior.

Verify successful HTTP-to-broker-to-consumer flow, DTO failures, reader invalidation, redelivery policy, ACK/NACK and shutdown. Publisher confirmation and unroutable RabbitMQ delivery need actual brokers. Native platform repositories separately test their own transports.

For public-module verification, use a new module cache, `GOWORK=off` and a separate consumer without local replacements. Record the exact core/platform versions and commits. Do not call a source workspace result a completed release combination.

See [event contracts](../reference/event-platform.md), [publishing](../migration/app-publisher.md) and [release operations](../maintenance/releasing.md).

[Korean original](../ko/testing/platforms.md).
