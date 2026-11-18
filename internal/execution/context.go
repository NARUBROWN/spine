package execution

import (
	"context"
	"mime/multipart"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
)

// Context owns the state and HTTP input of one pipeline execution.
// Internal consumers use its methods; public callers receive a separate view.
type Context struct {
	state State
	input Input
}

// NewContext initializes one execution and owns its state and input.
func NewContext(ctx context.Context, sources Sources) *Context {
	if ctx == nil {
		panic("execution context is nil")
	}
	return &Context{state: State{ctx: ctx}, input: Input{sources: sources}}
}

// NewHTTPContext initializes an execution from a single HTTP transport scope.
func NewHTTPContext(request interface {
	HTTPRequestContext
	HTTPResponseContext
	RoutingContext
}) *Context {
	return NewContext(request.Context(), Sources{
		Routing: request, HTTPRequest: request, HTTPResponse: request,
	})
}

func (c *Context) Context() context.Context          { return c.state.ctx }
func (c *Context) Get(key string) (any, bool)        { return c.state.Get(key) }
func (c *Context) Set(key string, value any)         { c.state.Set(key, value) }
func (c *Context) Routing() RoutingContext           { return c.input.Routing() }
func (c *Context) HTTPRequest() HTTPRequestContext   { return c.input.HTTPRequest() }
func (c *Context) HTTPResponse() HTTPResponseContext { return c.input.HTTPResponse() }

// Close runs after completion hooks and revokes retained views. It is idempotent.
// HTTP transport cleanup follows this operation in the transport owner.
func (c *Context) Close() {
	c.state.ReleaseStore()
	c.input.Close()
}

func (c *Context) readInput(read func(Sources)) { c.input.read(read) }

// RoutingContext is the HTTP router's input, not a common execution requirement.
type RoutingContext interface {
	Method() string
	Path() string
	SetRoute(map[string]string, []string)
}

// State owns only per-execution values and the caller's cancellation context.
type State struct {
	Store
	ctx context.Context
}

type HTTPRequestContext interface {
	Context() context.Context
	Method() string
	Path() string
	PathKeys() []string
	Param(name string) string
	Query(name string) string
	Header(name string) string
	Params() map[string]string
	Queries() map[string][]string
	Headers() map[string][]string
	Bind(out any, allowedMedia ...[]string) error
	MultipartForm() (*multipart.Form, error)
}

type HTTPResponseContext interface {
	ResponseWriter() httpcontract.ResponseWriter
	SetResponseWriter(httpcontract.ResponseWriter)
}
