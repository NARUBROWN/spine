# Register HTTP routes in controllers

Implement `RegisterRoutes(*api.Registry)` on a DI-managed controller. Put the HTTP callback directly in each route declaration and use explicit input wrappers.

```go
func (c *UserController) RegisterRoutes(r *api.Registry) {

    r.GET("/users/:id", func(ctx spine.Ctx, path input.Path[UserPath]) (httpx.Response[User], error) {
        user, err := c.service.Find(ctx.ExecutionContext().Context(), path.ID)
        return httpx.Response[User]{Body: user}, err
    }, api.Tags("users"), api.Summary("Read a user"))

}
```

The sample assumes `UserPath`, `User`, and a service with `Find(context.Context, int64)` are defined. The callback owns HTTP concerns; the service takes standard context and explicit values.

Register the controller with `app.Constructor(constructor.Controller(&UserController{}))`. Its `Constructor` resolves dependencies. A `constructor.Custom` factory is not collected as a controller even if its output happens to implement `api.Controller`.

Registry and Group calls use `path, callback, options...`. Apply `api.Tags`, `api.Summary`, `api.Description` and `api.Interceptors` directly. Group prefixes and `HTTP.GlobalPrefix` compose with registered paths. Do not use removed `api.OptionSet` or `api.Options` wrappers.

Use `spine.Ctx` at the HTTP entry point. The resolver supplies a restricted view over the request's execution owner. Request parameters, queries, headers and bodies use `input.Path`, `input.Query`, `input.Header` and `input.Body`. Unsupported return values, duplicate input keys and conflicting routes fail preparation.

The deprecated `app.Route` remains available for migration. New examples use controller registration. Runtime OpenAPI describes actual collected callbacks and excludes WS routes. See [constructors](component-contracts.md), [inputs](structured-inputs.md), [OpenAPI](../reference/openapi.md) and the [demo](../examples/demo.md).

[Korean original](../ko/migration/controller-routes.md).
