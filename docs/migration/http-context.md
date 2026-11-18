# HTTP execution context ownership

Each HTTP request now creates one `execution.Context`. Its constructor initializes
its private `State` and `Input`. The pipeline, argument resolvers, return handlers
and post-execution hooks receive that context instead of separate state and input
arguments. They access request data and response operations through the context's
methods. The context does not return its state or input objects.

The public HTTP contract remains `spine.Ctx`. Its method set comes from the
canonical `contextview.Ctx` contract. Handler and interceptor arguments receive a
separate view object that delegates permitted operations to the execution owner.
The view does not embed or return the internal context.

Applications can continue to read request information, get and set execution
values, and write responses through `HTTPResponseView`. Internal routing, body
binding, response writer replacement and execution cleanup are excluded from the
public views, including when inspected through interface type assertions.

Argument resolution selects the view from the handler's declared type, not its
parameter name. Declaring `context.Context` continues to receive the original Go
request context. Controllers and interceptors can receive separate view instances
that share the same execution owner.

Response preparation, interceptor ordering and flushing are preserved. After
`AfterCompletion`, `Context.Close` releases execution values and detaches the HTTP
input. Retained views cannot restore values or write to a completed response.
Transport cleanup then releases the body parser's resources and the response
writer. Closing the execution does not cancel the original Go request context;
its cancellation remains owned by the HTTP platform.
