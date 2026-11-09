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

## CORS policy

Register the CORS interceptor globally for HTTP before authentication so it can handle preflight requests. An allowed preflight returns 204; a denied preflight returns 403. Ordinary OPTIONS requests continue to the handler. CORS does not replace server-side authentication or authorization.

Set `ExposeHeaders` to expose response headers and `MaxAge` to a pointer to the preflight cache time in seconds. A nil `MaxAge` omits the header, zero emits `0`, and negative values are invalid. `AllowOrigin` is called only when the static origin list does not allow the request and returns `(bool, error)`. Callback errors are returned as request errors. An empty list without a callback allows no origins. A wildcard does not include the `null` origin; allow it explicitly or through the callback. Callbacks must match trusted scheme, host, and port boundaries.

Wildcard origins cannot be combined with credentials. Wildcard methods, request headers, and exposed headers do not act as wildcards for credentialed browser requests. Explicitly include `Authorization` in `AllowHeaders` even without credentials. The interceptor preserves existing `Vary` values and adds `Origin` even when the request has no Origin. OPTIONS responses also vary on the requested method and headers.

The added `Config` fields require callers using positional struct literals to switch to named fields. Previously denied preflights also returned 204 and ordinary responses included allow-methods and allow-headers. They now return 403 and the allow-methods and allow-headers are emitted on successful preflights. A CORS interceptor attached only to a GET route does not automatically apply to OPTIONS. Register a global policy or register OPTIONS and its policy explicitly. Automatic precedence between global and per-route policies is not provided. Do not apply both native Echo CORS and the common interceptor to the same request.
