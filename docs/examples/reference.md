# Run the platform reference examples

`cmd/reference` demonstrates HTTP contexts, file uploads, injected publishers, event consumers and WebSocket output. Broker integrations are disabled until explicitly configured.

```sh
go -C cmd/reference run .
```

Use a workspace selecting this core and compatible Echo, Gorilla, Kafka and RabbitMQ modules. The default address is `:8080`; the allowed browser origin defaults to `http://localhost:5173`.

| Route | Demonstrated behavior |
| --- | --- |
| `GET /users/:id` | DI-managed logging interceptor, shared execution value and an intentional 404 response. |
| `GET /users` | Explicit query DTO. |
| `POST /upload` | Multipart fields/files and optional query values. |
| `GET /headers` | Explicit header DTO. |
| `POST /orders/:orderId` | Publish `order.created` with `order_id`. |
| `POST /stocks/:stockId` | Publish `stock.created` with `stock_id`. |
| `GET /ws/chat` | Decode `input.Message[ChatMessage]` and reply through injected `ws.Sender`. |

Publishing uses `publish.Publisher` with the HTTP execution's standard context. Missing platforms produce 503. Send failures are returned. Consumers accept `spine.EventCtx`; the runtime owns settlement. HTTP interceptors are registered separately from WS hooks.

## Broker and proxy configuration

Use `SPINE_DEMO_KAFKA_BROKERS` for Kafka and `SPINE_DEMO_RABBITMQ_URL` for RabbitMQ. Production Kafka defaults to TLS. RabbitMQ requires AMQPS unless `SPINE_DEMO_ALLOW_INSECURE_BROKERS=true` explicitly enables local plaintext transport.

Optional `SPINE_DEMO_RABBITMQ_DLX` and `SPINE_DEMO_RABBITMQ_DLX_ROUTING_KEY` configure dead-letter topology. Provision bindings separately. Do not commit broker credentials.

`SPINE_DEMO_ALLOWED_ORIGIN` selects the browser origin. `SPINE_DEMO_TRUSTED_PROXY_CIDRS` must contain exact trusted proxy CIDRs; proxies must replace forwarded headers. Broker retry policy belongs to the consumer transport, not business retries.

## Focused examples

- [WebSocket echo and HTTP push](websocket.md): `go -C cmd/reference run ./websocket`.
- [Publish-only outbox relay](publish-only.md): `go -C cmd/reference run ./publish-only`.
- [Kafka publisher](kafka-publish.md): separate minimal Kafka application.
- [Security configuration](../security-configuration.md): construction and preflight without connecting.

```sh
go run ./scripts/test -module cmd/reference test -race ./... -count=1 -timeout=5m
```

Use a real broker separately to verify confirmations, routing failures and redelivery. See [platform tests](../testing/platforms.md).

[Korean original](../ko/examples/reference.md).
