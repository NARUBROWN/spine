# Security configuration reference

Spine uses secure transport and bounded runtime defaults. Call `App.Validate` in deployment checks; `App.Run` performs the same network-free preflight automatically.

## Defaults

| Area | Default | Explicit development or compatibility option |
|---|---|---|
| Kafka | implicit TLS 1.2+ for each enabled reader/writer | custom `TLS`/`Dialer`/`Transport`, or local-only `AllowInsecureTransport: true` |
| RabbitMQ | `amqps://` required | `AllowInsecureTransport: true` with `amqp://` |
| RabbitMQ publish | persistent, mandatory routing, publisher confirms | tune `OperationTimeout`; failed messages are not automatically resent |
| RabbitMQ handler failure | reject without requeue | `FailurePolicy: rabbitmqplatform.FailureRequeue` |
| RabbitMQ prefetch | one unacknowledged message per consumer | positive `PrefetchCount` |
| Consumer transport failure | rebuild reader with exponential backoff and jitter | configure `spine.WithEventConsumerRetry` |
| WebSocket origin | scheme and host must match | exact `AllowedOrigins` |
| WebSocket capacity | 1024 active and pending connections | positive limit or `UnlimitedWebSocketConnections` |
| WebSocket authentication | implementations of `WebSocketHandshakeInterceptor` run before slot/upgrade | implement handshake auth on every protected WS route |
| Global interceptors | HTTP and WebSocket | `InterceptorFor` with a narrower scope |
| Credentialed CORS | explicit origins required | none; wildcard credentials are invalid |
| Cookie serialization | reject invalid fields before writing a response | `EncodeCookieValue` for arbitrary values |

## Operational requirements

- Only enable insecure broker transport in an isolated local environment.
- Provision a RabbitMQ dead-letter exchange before configuring it on a Spine consumer queue.
- Make Kafka/RabbitMQ consumers idempotent. Reconnects and lost confirms can legitimately cause at-least-once redelivery or duplicates.
- When enabling trusted proxy CIDRs, configure the proxy to remove or overwrite client-supplied forwarding headers.
- Alert on `WEBSOCKET_CAPACITY_EXCEEDED`, consumer reconnect exhaustion, and event publish failures.
- Treat `boot.ConfigError.Issues` codes as stable machine-readable deployment diagnostics; do not parse the human message.

See the [v0.5.1 corrective guide](migration/v0.5.1.en.md), [v0.5 migration guide](migration/v0.5.en.md), and [compilable example](../examples/security-config/main.go).

See the [event platform migration guide](migration/event-platforms.en.md) for App registration and removed boot options. Broker transport settings belong to each platform Config.
