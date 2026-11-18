# Handler contexts and ownership

The runtime owns execution and cleanup. Handler contexts expose only application operations. Shared values are scoped to one HTTP request, WS frame or event delivery.

| Transport | Actual injected object | Public parameter | Owner |
| --- | --- | --- | --- |
| HTTP | `execution.publicContext`, a separate view | `spine.Ctx` | HTTP pipeline and `execution.Context` |
| WebSocket | Private `ws.messageContext` | `spine.WebSocketCtx` or `context.Context` | WS runtime, one context per frame |
| Event | Private `consumer.messageContext` | `spine.EventCtx` | Consumer runtime, one context per delivery |

## HTTP views

`execution.Context` owns State and Input. Internal consumers directly use `ctx.Input.HTTPRequest`, `ctx.Input.HTTPResponse` and `ctx.Input.Routing`. Handler resolution creates a separate public view, rather than passing the internal owner under a narrower interface.

```go
func(ctx spine.Ctx) error {
    ctx.ExecutionContext().Set("requestID", "example")
    request, ok := ctx.HTTPRequestContext()
    if !ok { return errors.New("HTTP request is unavailable") }
    response, ok := ctx.HTTPResponseContext()
    if !ok { return errors.New("HTTP response is unavailable") }
    response.SetHeader("X-Request-ID", request.Header("X-Request-ID"))
    return response.WriteString(200, "ok")
}
```

The fragment assumes `errors` and the public `spine` context package are imported. Prefer response return values for ordinary endpoints. The view still supports output for interceptors and explicit response handling.

Public HTTP views exclude routing setters, internal body Bind, native writer access/replacement and Close. Each request/response view operation holds the input read lock. After completion hooks, the owner closes the store and clears Input under the write lock. Transport cleanup follows. Retained views cannot restore values or write later. Standard context is the original request context; the platform owns its cancellation.

## WS and event contexts

Both expose `context.Context` methods plus Get and Set. Their private stores and cancel functions remain runtime-owned. They contain no exported Close, Cancel, Stop, ReleaseStore or settlement methods. A second wrapper is unnecessary while this public method boundary stays small; regression tests inspect the actual injected objects through type assertions and public method sets.

WS completes frame hooks before releasing the store, revoking the deprecated connection-local send capability and canceling the frame context. This does not cancel the connection. Event execution clears values and cancels its context before runtime ACK/NACK.

Read a standard HTTP context with `ctx.ExecutionContext().Context()`. A WS or event context can be passed directly to application services expecting `context.Context`. Services should take explicit inputs instead of querying framework-specific HTTP views.

## Background tasks

A request/frame/delivery context ends with that execution. Background work needs its own owner, cancellation and join. Constructors only assemble dependencies. Start workers explicitly after preparation and use shutdown hooks to cancel and join them before closing shared publishers. Do not retain request bodies, files or runtime input buffers for asynchronous work.

See [HTTP ownership migration](../migration/http-context.md), [WS hooks](../migration/websocket-dx.md) and [publish-only example](../examples/publish-only.md).
