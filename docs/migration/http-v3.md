# Migrate to HTTP v3

HTTP v3 gives the core raw request data. The platform no longer performs routing parameter conversion, DTO binding or multipart parsing before authentication.

Platforms implement `Context`, `Method`, `RawPath`, `RawQuery`, `Header`, `Headers`, `Body` and `Writer`. Preserve raw encoding and stream ownership. `WriteJSON` accepts serialized bytes. HTTP v2 factories cannot satisfy this contract through compatibility imports alone.

Application input uses `input.Path[T]`, `input.Query[T]`, `input.Header[T]` and `input.Body[T]`. Read standard context with `ctx.ExecutionContext().Context()`. Do not pass native framework contexts into application services.

## Preparation and servers

```go
prepared, err := app.Prepare(ctx, options)
if err != nil { return err }
handler, err := spine.NetHTTPHandler(prepared)
if err != nil { return err }
```

Connect the handler to a caller-owned net/http server, or use the prepared adapter's managed `Serve`. Always close the prepared application. Shutdown stops admission, drains active execution and then cleans resources. The caller's close context limits waiting rather than abandoning cleanup.

## Managed HTTPS

Load certificates with `tls.LoadX509KeyPair` and set `boot.HTTPOptions.TLS`. The prepared adapter uses it as its default. `Serve(listener, httpcontract.WithTLS(config))` can override it for that run. Supply a plaintext TCP listener. Invalid explicit TLS is rejected before native initialization; external-server TLS remains caller-owned.

Zero body limits select bounded defaults, negative body limits explicitly disable the bound, and multipart memory is a separate parser threshold. See [HTTP platform details](../reference/http-platform.md), [request parsing](../reference/http-inputs.md) and [source context ownership](http-context.md).

[Korean original](../ko/migration/http-v3.md).
