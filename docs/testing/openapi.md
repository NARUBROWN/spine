# Verify runtime OpenAPI

The `test/openapi` module compares generated descriptions and schemas with actual HTTP behavior. Select the current core, Echo platform and validator modules in a local workspace.

```sh
go -C test/openapi test -race ./... -count=1
```

The suite covers explicit input sources, nested validation, conditional required inputs, body media types, runtime opt-in, paths-only descriptions without a validator, resource conflicts and serving through real HTTP.

Documentation resources bypass business interceptors. Static inspection before controller collection and prepared runtime descriptions are separate phases. Compare each export against the phase and validator capabilities that produced it.

See [runtime OpenAPI](../reference/openapi.md), [embedded UIs](../reference/openapi-ui.md) and [test overview](README.md).

[Korean original](../ko/testing/openapi.md).
