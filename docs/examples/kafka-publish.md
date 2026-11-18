# Publish an order through Kafka

`examples/kafka-publish` injects `publish.Publisher` into `OrderService`. The HTTP callback passes its standard context and explicit order ID to the service.

Use compatible core and Kafka source modules in a workspace, then run:

```sh
KAFKA_BROKERS=localhost:19092 KAFKA_INSECURE=true go -C examples/kafka-publish run .
curl -X POST http://127.0.0.1:8080/orders/42
```

The default address is `127.0.0.1:8080`; override it with `HTTP_ADDRESS`. `KAFKA_TOPIC_PREFIX` maps the logical `orders.created` destination to the native topic. The destination catalog is shared with factory startup validation. TLS remains the default; plaintext requires the explicit local opt-in.

The HTTP response is 202 only after the synchronous publish succeeds. An error is propagated to HTTP handling. This example does not store business data transactionally with the event. See [DI publishing](../migration/app-publisher.md) and [outbox boundaries](publish-only.md).
