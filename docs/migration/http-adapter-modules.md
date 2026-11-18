# Select an independent HTTP platform

The core module has no concrete HTTP engine dependency. Select an HTTP platform explicitly:

```go
app := spine.New(echoPlatform.Option())
```

Import `echoPlatform` from `github.com/NARUBROWN/spine-echo-platform`. The independent `spine-nethttp-platform` is another implementation of the [HTTP v3 contract](../reference/http-platform.md). A custom factory can use `spine.WithHTTPAdapter`.

Native middleware, routes and framework hooks belong to the selected platform. Shared routing, authentication ordering, DTO parsing, validation and response handling belong to the core. Do not import a native HTTP engine into core contracts.

`App.Validate` checks composition without opening network resources. `Prepare` creates resources without starting the managed listener. A prepared app can supply a protected handler to an external server, or own a managed listener through its HTTP adapter. Preserve shutdown ownership when using an external listener.

Use matching core/platform source revisions or compatible released versions. A successful local workspace build is not evidence of public-tag installation. See [HTTP v3 migration](http-v3.md), [demo setup](../examples/demo.md) and [platform verification](../testing/http-platforms.md).

[Korean original](../ko/migration/http-adapter-modules.md).
