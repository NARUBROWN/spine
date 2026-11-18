# Run the HTTP and WebSocket demo

`cmd/demo` is the broker-free reference for application composition, CLI inspection and network E2E tests. It uses Echo for HTTP and Gorilla for WS.

Use a development workspace selecting this core checkout, the demo module and compatible platform modules. The module's local replacements require the sibling Echo checkout. From the repository root:

```sh
go -C cmd/demo run .
```

The default address is `127.0.0.1:8080`. `SPINE_PORT` changes the port; `SPINE_DEMO_ADDRESS` overrides the full address.

| Request | Result |
| --- | --- |
| `GET /health` | `{"status":"ok"}` |
| `GET /users/42` | `{"id":42}` |
| `POST /echo` with a JSON message | The decoded message and count. |
| `GET /ws/echo` | WS upgrade followed by raw message echo. |

```sh
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/users/42
curl -H 'Content-Type: application/json' -d '{"message":"hello","count":1}' http://127.0.0.1:8080/echo
```

HTTP callbacks declare `spine.Ctx` and explicit input wrappers. The WS callback declares `spine.WebSocketCtx`, `ws.ConnectionID` and `input.Message[[]byte]`; it sends through injected `ws.Sender`. The body limit is 1024 bytes. Invalid input, oversized bodies and unsupported media retain 400, 413 and 415 behavior.

`application.BuildApplication()` assembles dependencies without opening listeners. The server and CLI use the same composition root. The source-candidate CLI must understand WS callbacks, the application Sender and current core contracts. A published older CLI may reject these source APIs.

```sh
go run ./scripts/test -module cmd/demo test -race ./... -count=1 -timeout=5m
```

Tests cover actual HTTP/WS traffic, preparation, SIGINT shutdown, native hooks and rejection after closure. Unix signal tests are skipped on Windows. Source-workspace success does not prove an older public tag supports the new API. See [contexts](../guides/contexts.md) and the [reference application](reference.md).

[Korean original](../ko/examples/demo.md).
