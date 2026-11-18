# Compare raw HTTP contract implementations

`test/http-contract` contains verification probes for net/http and fasthttp. They are minimal native adapters for contract comparison, not product platforms or a net/http bridge.

Run the strict-stream verifier from the repository root:

```sh
go -C test/http-contract run ./cmd/strictstream --output-dir /tmp/spine-http-eof-proof
```

It reproduces known premature-chunk-termination failures in the original fasthttp v1.77.0, copies the dependency and probes into a temporary tree, applies the EOF patch and reruns the same tests. It verifies test/core/module hashes and writes logs and summary evidence without editing the original dependency cache.

The patch changes incomplete chunk-size/trailer termination from `io.EOF` to `io.ErrUnexpectedEOF`. This is evidence for the local patched combination. Unmodified upstream v1.77.0 is not claimed to pass those streaming checks.

To select only the net/http and buffered probes:

```sh
go -C test/http-contract test -race -count=1 -run '/^(nethttp|fasthttp-buffered)$' -timeout=90s ./...
```

That filter does not resolve excluded streaming failures. Preserve nil-body distinctions, reader getter no-read behavior, native option validation and connection reuse. For full Spine pipeline behavior, run [product platform tests](http-platforms.md). Historical detailed results are retained in the [Korean original](../ko/testing/http-contract.md).
