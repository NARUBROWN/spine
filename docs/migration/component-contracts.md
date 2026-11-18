# Register constructor roles

Use one `app.Constructor(...)` call with explicit role markers. A prototype's `Constructor(dependencies...) *Type` method assembles the object.

```go
app.Constructor(
    constructor.Repository(&UserRepository{}),
    constructor.Service(&UserService{}),
    constructor.Controller(&UserController{}),
    constructor.Interceptor(&LoggingInterceptor{}),
)
```

The types above must define their own Constructor methods. Use `constructor.Custom` for configuration, external resources, interface-returning factories and components without an HTTP role. Existing constructor functions remain valid where supported.

Controllers own `RegisterRoutes`. Services perform application work. Interceptors are created by DI, then selected separately for a route or global scope. To refer to a managed route interceptor, declare a typed nil pointer and pass it to `api.Interceptors`.

```go
var logging *LoggingInterceptor
r.GET("/users", func(ctx spine.Ctx) httpx.Response[string] {
    return httpx.Response[string]{Body: "ok"}
}, api.Interceptors(logging))
```

Preflight checks role contracts, constructor signatures, duplicate providers and dependency cycles without executing constructors. Runtime initialization still resolves actual dependencies and can return constructor errors.

`publish.Publisher` and `ws.Sender` are application-provided interfaces. Inject them and do not register replacement providers. Structural interface matching must remain unambiguous. Use [shutdown hooks](../guides/contexts.md) to cancel and join application-owned background work.

[Korean original](../ko/migration/component-contracts.md).
