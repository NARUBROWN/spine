package echo

import (
	"context"
	"maps"
	"mime/multipart"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/event/publish"
	"github.com/labstack/echo/v4"
)

type echoContext struct {
	echo     echo.Context
	reqCtx   context.Context
	store    map[string]any
	eventBus publish.EventBus
}

func NewContext(c echo.Context) core.ExecutionContext {
	return &echoContext{
		echo:     c,
		reqCtx:   c.Request().Context(), // 요청시 생성되는 Context
		store:    make(map[string]any),
		eventBus: publish.NewEventBus(),
	}
}

func (e *echoContext) Context() context.Context {
	return e.reqCtx
}

func (e *echoContext) Request() core.RequestContext {
	return e
}

func (e *echoContext) Bind(out any) error {
	return e.echo.Bind(out)
}

func (e *echoContext) Get(key string) (any, bool) {
	value, ok := e.store[key]
	return value, ok
}

func (e *echoContext) Header(name string) string {
	return e.echo.Request().Header.Get(name)
}

// Headers return a map of all headers in the request.
func (e *echoContext) Headers() map[string][]string {
	return e.echo.Request().Header
}

func (e *echoContext) Param(name string) string {
	if raw, ok := e.store["spine.params"]; ok {
		if m, ok := raw.(map[string]string); ok {
			if v, ok := m[name]; ok {
				return v
			}
		}
	}
	return e.echo.Param(name)
}

func (e *echoContext) Query(name string) string {
	return e.echo.QueryParam(name)
}

func (e *echoContext) Set(key string, value any) {
	e.store[key] = value
}

func (e *echoContext) JSON(code int, value any) error {
	return e.echo.JSON(code, value)
}

func (e *echoContext) String(code int, value string) error {
	return e.echo.String(code, value)
}

func (e *echoContext) Params() map[string]string {
	if raw, ok := e.store["spine.params"]; ok {
		if m, ok := raw.(map[string]string); ok {
			// return a shallow copy to avoid mutation
			copyMap := make(map[string]string, len(m))
			maps.Copy(copyMap, m)
			return copyMap
		}
	}

	names := e.echo.ParamNames()
	values := e.echo.ParamValues()

	params := make(map[string]string, len(names))

	for i, name := range names {
		if i < len(values) {
			params[name] = values[i]
		}
	}

	return params
}

func (e *echoContext) Queries() map[string][]string {
	return e.echo.QueryParams()
}

func (e *echoContext) Method() string {
	return e.echo.Request().Method
}

func (e *echoContext) Path() string {
	return e.echo.Request().URL.Path
}

func (e *echoContext) PathKeys() []string {
	if v, ok := e.store["spine.pathKeys"]; ok {
		if keys, ok := v.([]string); ok {
			return keys
		}
	}
	return nil
}

func (e *echoContext) MultipartForm() (*multipart.Form, error) {
	return e.echo.MultipartForm()
}

func (c *echoContext) EventBus() publish.EventBus {
	return c.eventBus
}
