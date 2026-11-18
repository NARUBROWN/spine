# HTTP input parsing

HTTP platforms provide raw input. The core parses it after authentication. Select the input source explicitly in the handler signature.

| Parameter | Source |
| --- | --- |
| `input.Path[T]` | Route variables; struct or supported scalar. |
| `input.Query[T]` | Query fields, including repeated values. |
| `input.Header[T]` | Header fields, matched without case sensitivity. |
| `input.Body[T]` | JSON, XML, URL-encoded form or multipart body. |

Struct wrappers expose fields directly. Scalar wrappers are dereferenced. Plain HTTP DTO parameters are rejected with `HTTP_INPUT_WRAPPER_REQUIRED`. Contexts and response types are not input DTOs.

```go
type Search struct {
    Query string `query:"q"`
    Limit *int   `query:"limit"`
}

r.GET("/search", func(ctx spine.Ctx, query input.Query[Search]) httpx.Response[string] {
    return httpx.Response[string]{Body: query.Query}
})
```

Use pointers to distinguish missing values from explicit zero or false. Field tags select external names. Repeated query fields can fill slices. A scalar `input.Path[T]` consumes the corresponding route key in declaration order; validate route/input agreement before serving.

The parser is created lazily and shares decoded request data across compatible arguments. Authentication denial must not read the body. The platform enforces the total body limit; multipart memory controls spill behavior separately. Invalid conversion or malformed bodies return 400, oversized bodies 413 and unsupported body media 415. Preserve cancellation and ordinary I/O error identity.

Multipart DTOs can contain `*multipart.FileHeader` or `[]*multipart.FileHeader` with `form` tags. Open and close files during the request. Parser-created temporary files are removed after completion hooks. Persist data needed by background work before returning.

Request views return copies of maps, slices and multipart field/header metadata. They do not expose internal `Bind` or mutable routing. See [structured input migration](../migration/structured-inputs.md) and [HTTP context ownership](../migration/http-context.md).

[Korean original](../ko/reference/http-inputs.md).
