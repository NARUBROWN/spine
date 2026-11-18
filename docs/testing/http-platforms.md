# Compare HTTP platforms

The `test/http-platform` module runs the same controllers and inputs over real Echo and net/http TCP servers. It covers HTTP/WS/OpenAPI composition, preparation and managed/external shutdown ownership.

```sh
go run ./scripts/httpverify platforms --nethttp-root ../spine-nethttp-platform --echo-root ../spine-echo-platform --output-dir /tmp/spine-http-platform-proof
```

Run from the core root. The verifier uses temporary modfiles and local replacements, runs race and vet, and checks that tracked module manifests remain unchanged. Use a toolchain that satisfies the selected validator and platform modules.

Checks include JSON/XML/form inputs, multiple scalar paths, repeated query fields, Host headers, authentication before body reads, body limits, multipart file lifetime, public context boundaries, output staging and cleanup after success/error/panic. WS tests check upgrades, frame handling and shutdown. TLS tests check certificates, trust failures, limits, disconnect cancellation and native drain.

Public-context tests inspect actual injected objects for lifecycle-control leaks through type assertions. HTTP allows Get/Set and response output but excludes routing replacement, native writers and execution cleanup.

Results are local source evidence. They do not prove HTTP/2 behavior, production load, public-tag installation or broker confirmation. See [context ownership](../guides/contexts.md) and [test overview](README.md).

[Korean original](../ko/testing/http-platforms.md).
