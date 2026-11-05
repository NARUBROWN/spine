package transport

import (
	"context"
	"errors"
	"maps"
	"mime/multipart"
	"net/url"
	"slices"

	adapter "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/event/publish"
	"github.com/NARUBROWN/spine/internal/input"
	"github.com/NARUBROWN/spine/pkg/httperr"
)

// HTTPContext는 독립 HTTP scope에 Spine 요청 store와 이벤트 버스를 결합합니다.
type HTTPContext struct {
	scope                adapter.HTTPContext
	store                map[string]any
	bus                  core.EventBus
	writer               core.ResponseWriter
	params               map[string]string
	pathKeys             []string
	multipartMemoryBytes int64
	body                 *input.Body
	bodyErr              error
	bodyReady            bool
	closed               bool
}

var _ core.ExecutionContext = (*HTTPContext)(nil)
var _ core.HttpRequestContext = (*HTTPContext)(nil)

func NewHTTPContext(scope adapter.HTTPContext, multipartMemoryBytes int64) *HTTPContext {
	return &HTTPContext{scope: scope, store: make(map[string]any), writer: scope.Writer(), multipartMemoryBytes: multipartMemoryBytes}
}

func (c *HTTPContext) ResponseWriter() core.ResponseWriter          { return c.writer }
func (c *HTTPContext) SetResponseWriter(writer core.ResponseWriter) { c.writer = writer }
func (c *HTTPContext) SetRoute(params map[string]string, pathKeys []string) {
	c.params = maps.Clone(params)
	c.pathKeys = slices.Clone(pathKeys)
}

func (c *HTTPContext) Context() context.Context { return c.scope.Context() }
func (c *HTTPContext) Method() string           { return c.scope.Method() }

// Path는 native 정리 없이 원시 경로를 한 번 디코딩합니다.
func (c *HTTPContext) Path() string {
	path, err := url.PathUnescape(c.scope.RawPath())
	if err != nil {
		return c.scope.RawPath()
	}
	return path
}
func (c *HTTPContext) Query(name string) string {
	values, _ := url.ParseQuery(c.scope.RawQuery())
	return values.Get(name)
}
func (c *HTTPContext) Queries() map[string][]string {
	values, _ := url.ParseQuery(c.scope.RawQuery())
	return values
}
func (c *HTTPContext) RawQuery() string { return c.scope.RawQuery() }

func (c *HTTPContext) Header(name string) string    { return c.scope.Header(name) }
func (c *HTTPContext) Headers() map[string][]string { return c.scope.Headers() }
func (c *HTTPContext) Set(key string, value any)    { c.store[key] = value }
func (c *HTTPContext) Get(key string) (any, bool)   { value, ok := c.store[key]; return value, ok }
func (c *HTTPContext) Bind(out any) error {
	if err := c.prepareBody(); err != nil {
		return err
	}
	return normalizeInputError(c.body.Bind(out))
}

func (c *HTTPContext) prepareBody() error {
	if c.closed {
		return input.ErrBodyClosed
	}
	if !c.bodyReady {
		c.bodyReady = true
		// 전체 본문 한도는 플랫폼 reader가 적용합니다. 공통 파서는 중복 제한하지 않습니다.
		c.body, c.bodyErr = input.NewBody(c.Context(), c.scope.Body(), c.Header("Content-Type"), input.BodyOptions{MaxBytes: -1, MultipartMemory: c.multipartMemoryBytes})
	}
	return c.bodyErr
}

// Cleanup은 완료 훅 뒤 공통 파서가 만든 파일만 회수합니다. 원본 스트림은 플랫폼 소유입니다.
func (c *HTTPContext) Cleanup() error {
	c.closed = true
	if c.body == nil {
		return nil
	}
	return c.body.Cleanup()
}

func (c *HTTPContext) MultipartForm() (*multipart.Form, error) {
	if err := c.prepareBody(); err != nil {
		return nil, err
	}
	form, err := c.body.MultipartForm()
	return form, normalizeInputError(err)
}

func (c *HTTPContext) Param(name string) string {
	if c.params != nil {
		if value, exists := c.params[name]; exists {
			return value
		}
	}
	return ""
}

func (c *HTTPContext) Params() map[string]string {
	if c.params != nil {
		return maps.Clone(c.params)
	}
	return nil
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
