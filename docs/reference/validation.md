# DTO validation contract

`contract/validator` defines `spine.validation/v1`. Choose a validation engine with `spine.WithValidator(engine)`. Omitting the option keeps binding enabled without validation. Nil and typed nil validators fail preflight with `ErrInvalidValidator`.

```go
type Validator interface {
    Validate(ctx context.Context, value any) error
}
```

Validation runs synchronously after binding and before the handler. HTTP authentication runs before body decoding. Binding failures do not invoke validation. HTTP supplies decoded DTO pointers; WS and event consumers validate the decoded DTO value, rather than an input wrapper.

A validator must support concurrent calls, preserve cancellation, avoid input mutation and avoid retaining DTOs after returning. Configure rules before serving. Unsupported values return `ErrInvalidValue`.

| Result | Runtime treatment |
| --- | --- |
| `nil` | Continue to the handler. |
| Non-empty `*validation.Error` | HTTP 400 with fields, WS 1008, or event NACK. |
| Invalid validator or other execution failure | HTTP 500, WS 1011, or event failure handling. |

`FieldError` contains `Field`, `Rule`, optional `Param` and optional `Message`. Field paths use external names as JSON Pointers. Escape `~` as `~0` and `/` as `~1`. Keep deterministic error ordering and do not include rejected values automatically.

`NewError(fields...)` copies its field list and returns nil when empty. `AsError` recognizes wrapped rule violations; it does not classify mixed cancellation or implementation failures as DTO rule violations. Empty or typed nil validation errors violate the validator contract.

`SchemaDescriber` supplies side-effect-free validation descriptions to runtime OpenAPI. `CheckValidator` validates registration without running DTO validation. If cancellation happens during validation, preserve it along with the original failure.

See [OpenAPI](openapi.md), [input wrappers](http-inputs.md) and [HTTP examples](../examples/demo.md).

[Korean original](../ko/reference/validation.md).
