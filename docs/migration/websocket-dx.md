# WebSocket callback registration and application Sender

Controllers can register `r.WS(path, callback, options...)` in `RegisterRoutes`.
`r.Group` supports the same operation. Group prefixes, the HTTP global prefix,
and `api.Interceptors` apply. Paths remain static. HTTP GET routes and OpenAPI
resources cannot overlap a WebSocket endpoint. WebSocket callbacks are excluded
from the HTTP OpenAPI document.

The callback runs once per incoming message, after upgrade. It accepts
`spine.Ctx`, `context.Context`, `ws.ConnectionID` and explicit
`input.Message[T]` parameters, and returns only errors. Use `Message[DTO]` for
JSON objects and `Message[[]byte]` for raw text or binary bytes. Read the decoded
value through `.Value`. Plain DTO parameters in new callbacks are rejected.
`PreHandle` runs before decoding and the registered validator receives the DTO,
not its wrapper. Existing `app.WebSocket().Register` method expressions remain
available for migration.

```go
func (c *ChatController) RegisterRoutes(r *api.Registry) {

    r.WS("/ws", func(sc spine.Ctx, id ws.ConnectionID, message input.Message[ChatMessage]) error {
        return c.service.Send(sc.ExecutionContext().Context(), id.Value, message.Value)
    })

}
```

Spine supplies one `ws.Sender` to the DI container before resolving controllers.
Do not register a Sender constructor. Services receive it in their Constructor.
It is available from HTTP, WebSocket handlers and independent background work.
It addresses existing connections by their opaque connection ID. It never opens
connections or broadcasts implicitly. IDs are process-local connection handles,
not authenticated user identities; the application authorizes recipient selection.

```go
type ChatService struct { sender ws.Sender }
func (*ChatService) Constructor(sender ws.Sender) *ChatService {
    return &ChatService{sender: sender}
}
// After serializing the application's event:
// return s.sender.Send(ctx, connectionID, ws.TextMessage, payload)
```

Send accepts text and binary frames, serializes writes on each connection and
waits for the platform write result. Success does not acknowledge client receipt
or client application processing. Errors and cancellation can follow a completed
write, so retrying can duplicate messages. Do not mutate payload during Send.
The call context controls admission and waiting for the connection's write lock.
The native write deadline is the earlier of the call deadline and server
WriteTimeout. An already-running native write without context cancellation
support completes under that deadline; canceling one caller does not close the
shared target connection.

Unknown and disconnected IDs return `ws.ErrConnectionNotFound`. Application
shutdown rejects new sends with `ws.ErrSenderClosed`; native failures remain
observable. A canceled request only affects its send call. Background work uses
its own context and must be canceled and awaited by its owner. Existing
`ws.Send(ctx, ...)` remains a connection-local migration API; a missing sender
now returns ErrConnectionNotFound instead of reporting success.

The common execution state now contains only the standard context and the
per-execution value store. HTTP input/output, a connection's immutable handshake
snapshot, and the received WS or consumer message travel separately to the
pipeline and its resolvers. No WS/consumer execution-context adapter remains.

A WS connection owns its session until disconnect. Each received frame gets a
child context and an input lease for one pipeline execution. Completion hooks
run before the store and input references are cleared; then the child context is
canceled. A retained public transport view becomes unavailable after completion.
Use `input.Message[T]` for received application data and explicitly copy any data
needed by owned background work. The legacy `ws.Send(ctx, ...)` capability is
also revoked at message completion. Injected `ws.Sender` remains usable with the
background owner's context while the target connection is active.

Consumer runtimes keep the delivery and ACK/NACK callbacks outside the pipeline.
They pass only subscription name, payload and metadata as input, and settle the
delivery after completion and per-message cancellation. HTTP still flushes its
staged response before completion hooks and releases its transport input before
parser cleanup. Connection-start and disconnect callbacks are not introduced;
existing handshake interceptors still run.

A runnable echo and HTTP-push example is in
[cmd/reference/websocket](../../cmd/reference/websocket/main.go). From
`cmd/reference`, run `go run ./websocket`. Open a local same-origin page, then:

```js
const socket = new WebSocket("ws://localhost:8080/ws");
socket.onopen = () => socket.send(JSON.stringify({ text: "hello" }));
socket.onmessage = event => console.log(JSON.parse(event.data));
```

The first reply contains `connectionId`. POST `{"text":"push"}` to
`/push/<connectionId>` to send to that same connection from an HTTP request.
