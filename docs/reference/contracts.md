# Platform contracts

Spine owns execution policy. Independent modules own HTTP engines, WebSocket connections, broker clients and validation engines. Public platform contracts live under `github.com/NARUBROWN/spine/contract` and use the Go standard library.

| Package | Responsibility | Application integration |
| --- | --- | --- |
| `contract/http` | Raw requests, response output and server lifetime | HTTP platform `Option()` or `WithHTTPAdapter` |
| `contract/websocket` | Upgrade, frames and connection I/O | WebSocket platform `Option()` or `WithWebSocketAdapter` |
| `contract/event` | Messages, readers, publishers and settlement | `WithEventAdapter` |
| `contract/validator` | DTO validation and field errors | `WithValidator` |
| `contract/description` | Registration and schema snapshots | `DescribeHTTP` and runtime OpenAPI |

Choose only the integrations the application uses. HTTP-less event workers do not need an HTTP or WebSocket platform. WebSocket routes require an HTTP platform with upgrade support and a separate WebSocket factory.

The `adapter`, `adapter/http`, `adapter/event` and `adapter/websocket` packages retain compatibility aliases. Use the canonical contract paths in new implementations. See [compatibility imports](adapter-compatibility.md).

Application handler contexts are separate from platform contracts: HTTP receives `spine.Ctx`, WebSocket receives `spine.WebSocketCtx` or `context.Context`, and consumers receive `spine.EventCtx`. See [execution contexts](../guides/contexts.md).

Read the [HTTP](http-platform.md), [WebSocket](websocket-platform.md), [event](event-platform.md), [validation](validation.md) and [OpenAPI](openapi.md) references. Run [platform integration tests](../testing/platforms.md) before claiming that a concrete module combination works.

[Korean original](../ko/reference/contracts.md).
