package echo

import (
	"context"

	"github.com/NARUBROWN/spine/core"
	"github.com/labstack/echo/v4"
)

type echoContext struct {
	echo   echo.Context
	reqCtx context.Context
	store  map[string]any
}

func NewContext(c echo.Context) core.ExecutionContext {
	return &echoContext{
		echo:   c,
		reqCtx: c.Request().Context(), // 요청시 생성되는 Context
		store:  make(map[string]any),
	}
}

func (e *echoContext) Context() context.Context {
	return e.reqCtx
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
			for k, v := range m {
				copyMap[k] = v
			}
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

func (c *echoContext) PathKeys() []string {
	if v, ok := c.store["spine.pathKeys"]; ok {
		if keys, ok := v.([]string); ok {
			return keys
		}
	}
	return nil
}
