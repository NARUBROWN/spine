# Runtime OpenAPI

Enable `boot.HTTPOptions.OpenAPI` to serve OpenAPI 3.1.2 generated from actual registrations and Go types. No CLI, source comments, Swaggo or runtime source files are required.

```go
options := boot.Options{
    Address: ":8080",
    HTTP: &boot.HTTPOptions{
        OpenAPI: boot.OpenAPIOptions{
            Enabled: true,
            Title: "Example API",
            Version: "1.0.0",
        },
    },
}
```

| Option | Default | Meaning |
| --- | --- | --- |
| `Enabled` | `false` | Generate and serve the document. |
| `Path` | `/openapi.json` | Static absolute path for GET and HEAD. |
| `Title` | `Spine API` | `info.title`. |
| `Version` | `unversioned` | Application API version. |

Setting a title or registering a UI does not enable generation. `HTTP: nil` disables HTTP resources. The document path is independent of the business global prefix. Enabled document paths must not overlap application GET, HEAD or WS routes, including parameterized overlaps.

Without a validator's schema descriptions, output is paths-only. Do not infer detailed inputs or responses from that mode. WS callbacks are excluded from HTTP OpenAPI.

`DescribeHTTP(app, options)` reads registrations without running constructors, handlers, external factories or validation rules. Collected controller callbacks can require preparation; `ErrRoutesNotCollected` reports that boundary. `openapi.Generate` creates bytes for an explicit export. Description snapshots and prepared documents are isolated from later mutation.

Document and UI requests pass platform middleware but bypass business handlers and Spine interceptors. Apply access restrictions to the JSON path and UI assets at the external server or proxy when needed.

Input planning, duplicate key detection, binding and schema generation share the same rules. Reject multiple bodies, duplicate input DTOs and conflicting keys before serving. Validate descriptions against actual requests with [OpenAPI tests](../testing/openapi.md). See [embedded UIs](openapi-ui.md).

[Korean original](../ko/reference/openapi.md).
