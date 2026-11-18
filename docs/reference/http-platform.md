# HTTP platform contract

`contract/http` defines `spine.http-adapter/v3`. Platforms provide raw input and response output. Spine owns routing, DTO conversion, validation, body parsing and pipeline execution.

## Raw request input

`HTTPContext` provides `Context`, `Method`, `RawPath`, `RawQuery`, `Header`, `Headers`, `Body` and `Writer`.

| Input | Required behavior |
| --- | --- |
| `RawPath` | Encoded path before native path cleaning; exclude query and authority. |
| `RawQuery` | Original query without `?`; preserve repeated and encoded values. |
| `Headers` | Preserve Host and port. Returned maps and slices must not mutate the request or follow reused native buffers. |
| `Body` | Return the same non-nil stream without reading, parsing or replaying it. The platform owns the original stream. |
| `Context` | Provide a safe request context, canceled when the request ends. Declare disconnect cancellation support separately. |

Authentication hooks run before Spine reads the body. Native protocol rejection before `Execute` is a separate boundary. Request data, reader and writer are borrowed for the synchronous request lifetime.

`ResponseWriter` supports headers, commitment checks, status, JSON, string and byte output. `WriteJSON` accepts serialized `json.RawMessage`. Writers must permit callers to reuse supplied bytes after a write returns. Successful output means logical commitment, not confirmed client receipt. Return observable write errors.

## Server ownership

| Operation | Required behavior |
| --- | --- |
| `Serve(net.Listener, ...ServeOption)` | Own and close the listener on every return path. Run once and normalize ordinary shutdown to `ErrServerClosed`. Unexpected nil return is invalid. |
| `StopAccepting()` | Synchronously reject new execution, remain idempotent and preserve active requests. Requests arriving after admission stops receive 503. |
| `Shutdown(context.Context)` | Stop admission and wait for active work. The supplied context limits waiting; cleanup continues and a later call can wait again. |

Track work at the outermost native handler, including middleware, native routes and upgrades. External servers own their listeners; the prepared application retains adapter shutdown ownership. Long-lived WebSocket connections are drained by the WS runtime. Starting shutdown must not cancel every active HTTP request.

## TLS and optional capabilities

`Config.Serve.TLS` supplies managed-server TLS defaults. Applications use `boot.HTTPOptions.TLS`; nil selects plaintext. Load certificates with `tls.LoadX509KeyPair` before preparation.

```go
certificate, err := tls.LoadX509KeyPair("server.crt", "server.key")
if err != nil { return err }
config := &tls.Config{Certificates: []tls.Certificate{certificate}}
err = prepared.HTTPAdapter().Serve(listener, httpcontract.WithTLS(config))
```

Supply a plaintext TCP listener. Serve options resolve in order, the last `WithTLS` wins, and `WithTLS(nil)` is invalid. TLS settings are cloned; private keys, callbacks and I/O providers remain shared. Static certificate and capability validation precedes native initialization. Dynamic certificate availability and client trust require real TLS tests.

`Capabilities` declares `TLS`, `NetHTTPHandler`, `NetHTTPUpgrade`, `ClientDisconnectCancellation` and individual `ServerOptions`. Validate the factory, requested config and upgrade routes before creating an adapter, then validate the returned adapter. Never silently ignore an unsupported explicit option.

`spine.NetHTTPHandler(prepared)` obtains an optional protected net/http handler. Unsupported capabilities return `ErrUnsupportedCapability`; invalid adapters and nil native handlers return `ErrInvalidAdapter`. A factory declaring upgrade support must implement the corresponding upgrade factory contract.

## Limits and errors

The core normalizes zero `MaxBodyBytes` to 32 MiB. Negative values disable the body limit. Zero native timeouts select platform defaults; negative timeouts are invalid. Zero multipart memory selects 32 MiB and negative multipart memory is invalid. Multipart memory is a parser threshold, not a total request-memory bound.

Use `ErrInvalidAdapter` for malformed factories or adapters, `ErrInvalidState` for lifecycle violations and `ErrUnsupportedOption` for unsupported explicit server options. `RequestError` preserves status, message and cause. Oversized bodies use 413. Do not classify cancellation and arbitrary I/O failures as malformed input.

See [HTTP inputs](http-inputs.md), [TLS and security](../security-configuration.md), [HTTP migration](../migration/http-v3.md) and [real platform tests](../testing/http-platforms.md).

[Korean original](../ko/reference/http-platform.md).
