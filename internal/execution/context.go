package execution

import (
	"context"
	"mime/multipart"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
)

// Context is the state shared by one pipeline execution.
// Transport inputs are passed separately in Input.
type Context interface {
	Context() context.Context
	Set(string, any)
	Get(string) (any, bool)
}

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

func NewState(ctx context.Context) *State {
	if ctx == nil {
		panic("execution context is nil")
	}
	return &State{ctx: ctx}
}
func (s *State) Context() context.Context { return s.ctx }

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
