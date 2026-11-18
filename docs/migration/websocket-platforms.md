# Select an independent WebSocket platform

Select HTTP and WebSocket implementations independently. `contract/websocket` defines `spine.websocket-adapter/v1`; the core has no concrete WebSocket dependency.

```go
app := spine.New(echoPlatform.Option(), gorillaPlatform.Option())
```

The HTTP platform must support upgrades. HTTP-only applications omit the Gorilla option, and HTTP-less workers omit both HTTP and WS. Missing or incompatible capabilities fail before initialization.

Register current callbacks with `r.WS` and inject `ws.Sender` for output. The runtime owns origin checks, connection admission, per-frame execution, serialized writes and graceful shutdown. It runs WS-specific message and upgrade hooks separately from the HTTP pipeline.

Use compatible core and platform revisions. A historical pseudo-version combination is not evidence for these newer source APIs. CLI dependency updates do not automatically insert platform options or migrate callbacks; verify the installed CLI's supported contract.

See [WS callbacks and hooks](websocket-dx.md), [platform contract](../reference/websocket-platform.md), [runnable example](../examples/websocket.md) and [platform tests](../testing/http-platforms.md).

[Korean original](../ko/migration/websocket-platforms.md).
