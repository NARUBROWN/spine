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

This change does not remove the internal WS execution adapter. A broader split
of common execution state, connection sessions and received input is separate
from the public callback/Sender API. Connection-start and disconnect callbacks
are not introduced by this change; existing handshake interceptors still run.

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
