package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/pkg/header"
)

// HeaderResolver preserves value injection for the deprecated header API.
type HeaderResolver struct{}

func (*HeaderResolver) Supports(pm ParameterMeta) bool {
	return pm.Type == reflect.TypeFor[header.Values]()
}

func (*HeaderResolver) Resolve(ctx *execution.Context, _ ParameterMeta) (any, error) {
	httpCtx := ctx.Input.HTTPRequest
	ok := httpCtx != nil
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}
	return header.NewValues(httpCtx.Headers()), nil
}
