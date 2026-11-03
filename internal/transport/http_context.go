package transport

import (
	"context"
	"errors"
	"maps"
	"mime/multipart"
	"slices"

	adapter "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/event/publish"
	"github.com/NARUBROWN/spine/pkg/httperr"
)

// HTTPContext는 독립 HTTP scope에 Spine 요청 store와 이벤트 버스를 결합합니다.
type HTTPContext struct {
	scope    adapter.HTTPContext
	store    map[string]any
	bus      core.EventBus
	writer   core.ResponseWriter
	params   map[string]string
	pathKeys []string
}

var _ core.ExecutionContext = (*HTTPContext)(nil)
var _ core.HttpRequestContext = (*HTTPContext)(nil)

func NewHTTPContext(scope adapter.HTTPContext) *HTTPContext {
	return &HTTPContext{scope: scope, store: make(map[string]any), writer: scope.Writer()}
}

func (c *HTTPContext) ResponseWriter() core.ResponseWriter          { return c.writer }
func (c *HTTPContext) SetResponseWriter(writer core.ResponseWriter) { c.writer = writer }
func (c *HTTPContext) SetRoute(params map[string]string, pathKeys []string) {
	c.params = maps.Clone(params)
	c.pathKeys = slices.Clone(pathKeys)
}

func (c *HTTPContext) Context() context.Context     { return c.scope.Context() }
func (c *HTTPContext) Method() string               { return c.scope.Method() }
func (c *HTTPContext) Path() string                 { return c.scope.Path() }
func (c *HTTPContext) Query(name string) string     { return c.scope.Query(name) }
func (c *HTTPContext) Queries() map[string][]string { return c.scope.Queries() }
func (c *HTTPContext) Header(name string) string    { return c.scope.Header(name) }
func (c *HTTPContext) Headers() map[string][]string { return c.scope.Headers() }
func (c *HTTPContext) Set(key string, value any)    { c.store[key] = value }
func (c *HTTPContext) Get(key string) (any, bool)   { value, ok := c.store[key]; return value, ok }
func (c *HTTPContext) Bind(out any) error           { return normalizeInputError(c.scope.Bind(out)) }

func (c *HTTPContext) MultipartForm() (*multipart.Form, error) {
	form, err := c.scope.MultipartForm()
	return form, normalizeInputError(err)
}

func (c *HTTPContext) Param(name string) string {
	if c.params != nil {
		if value, exists := c.params[name]; exists {
			return value
		}
	}
	return c.scope.Param(name)
}

func (c *HTTPContext) Params() map[string]string {
	if c.params != nil {
		return maps.Clone(c.params)
	}
	return c.scope.Params()
}

func (c *HTTPContext) PathKeys() []string {
	return slices.Clone(c.pathKeys)
}

func (c *HTTPContext) EventBus() core.EventBus {
	if c.bus == nil {
		c.bus = publish.NewEventBus()
	}
	return c.bus
}

func normalizeInputError(err error) error {
	var input *adapter.RequestError
	if errors.As(err, &input) {
		return &httperr.HTTPError{Status: input.Status, Message: input.Error(), Cause: err}
	}
	return err
}
