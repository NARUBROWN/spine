# WebSocket echo and HTTP push

Run `go -C cmd/reference run ./websocket` in the development workspace. The example registers `/ws` and `/push/:id` in one controller.

```js
const socket = new WebSocket("ws://localhost:8080/ws");
socket.onopen = () => socket.send(JSON.stringify({ text: "hello" }));
socket.onmessage = event => console.log(JSON.parse(event.data));
```

Use a same-origin page or configure the allowed origin. The reply contains `connectionId`. POST `{"text":"push"}` to `/push/<connectionId>` to address the same connection from HTTP.

The controller delegates to a service that accepts `context.Context`, a connection ID and an explicit DTO. DI supplies `ws.Sender`; the app does not register a Sender constructor. The WS callback can use `context.Context` when no local `Get/Set` is needed. HTTP obtains it through `ctx.ExecutionContext().Context()`.

Connection IDs are process-local handles. Authorize target selection in a real application. A successful send confirms the platform write, not client processing. See [WebSocket contexts and hooks](../migration/websocket-dx.md).
