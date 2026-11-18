# Embedded OpenAPI UIs

Swagger UI and ReDoc can be embedded in the Go binary. Serving documentation requires no npm, source checkout or CDN access.

```go
options := boot.Options{
    HTTP: &boot.HTTPOptions{
        OpenAPI: boot.OpenAPIOptions{
            Enabled: true,
            UI: []boot.OpenAPIUI{
                swaggerui.New(swaggerui.Options{}),
                redoc.New(redoc.Options{}),
            },
        },
    },
}
```

Import the providers from `pkg/openapi/ui/swaggerui` and `pkg/openapi/ui/redoc`. Only imported UI providers contribute assets. Swagger UI defaults to `/docs` with title `API documentation`; ReDoc defaults to `/redoc` with title `API reference`. JSON defaults to `/openapi.json`.

UI and asset paths are independent of `HTTP.GlobalPrefix`. Relative resource URLs support reverse proxies that strip an external prefix. When the specification has no explicit servers, the browser uses its deployment prefix. Explicit servers remain authoritative.

GET and HEAD serve UI pages and resources under `/_assets/`. These public resources bypass business interceptors. Restrict the JSON endpoint, UI paths and assets together if documentation is private. Adding a UI does not replace a validator's schema descriptions.

Preparation validates providers and captures the returned static resources. Providers must avoid external connections and user-handler execution. A separate `Validate` call invokes provider validation again.

## Maintaining bundled assets

Run commands from the repository root:

```sh
go run ./scripts/openapi-ui --check
go test ./scripts/openapi-ui
node --test scripts/openapi-ui/test_bundle.cjs
```

Follow the updater's help for upstream checks and approved updates. Recheck version-specific offline patches when changing assets. Automatic updates are constrained to reviewed Swagger UI 5.x and ReDoc 2.x releases; major-version changes need separate integration and browser validation. Preserve bundled license notices. See [attribution](../attribution.md) and [runtime OpenAPI](openapi.md).

[Korean original](../ko/reference/openapi-ui.md).
