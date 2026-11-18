# Migrate to explicit HTTP inputs

Use a source wrapper for every HTTP DTO. Struct field tags select external names; the wrapper selects where values come from.

```go
type UserPath struct { ID int64 `path:"id"` }
type Search struct { Name string `query:"name"` }
type RequestHeaders struct { RequestID string `header:"X-Request-ID"` }
type CreateUser struct { Name string `json:"name" form:"name"` }
```

Declare the corresponding callback arguments as `input.Path[UserPath]`, `input.Query[Search]`, `input.Header[RequestHeaders]` or `input.Body[CreateUser]`. Access fields directly. Use pointer fields when missing values differ from zero or false.

`input.Path[string]`, numeric and boolean scalar wrappers consume route variables in parameter order and are dereferenced. Check multiple path variables against the route plan. Duplicate route keys, unknown DTO keys and mismatched bindings fail preflight.

Query slices accept repeated values. Header names are case-insensitive; explicit tags are needed for names such as `X-Request-ID`. Default lower-camel field names are not automatically prefixed. Validation error paths use external names and canonical header spelling.

`input.Body[T]` handles JSON, XML, form and multipart according to Content-Type. Multipart file fields use `*multipart.FileHeader` or slices of them with `form` tags. Consume files during the request and persist data needed after completion. The public `MultipartForm` view supports dynamic field names with copied metadata.

Plain DTO pointers are rejected with `HTTP_INPUT_WRAPPER_REQUIRED`. Do not mix deprecated path/query/header wrappers with explicit DTO wrappers in one handler. Multiple distinct bodies, duplicate DTOs and overlapping source keys are rejected before serving.

Conversion failures return 400, excessive bodies 413 and unsupported media 415. Cancellation and general I/O failures retain their cause. Authentication runs before parsing. OpenAPI and actual binding share the input plan.

WS and event payloads have separate contracts: WS callbacks use `input.Message[T]`, while consumer DTOs are decoded from the delivered payload. See [HTTP input reference](../reference/http-inputs.md), [contexts](../guides/contexts.md) and [validation](../reference/validation.md).

[Korean original](../ko/migration/structured-inputs.md).
