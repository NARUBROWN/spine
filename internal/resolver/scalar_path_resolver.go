package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/input"
	"github.com/NARUBROWN/spine/pkg/httperr"
)

type ScalarPathResolver struct{}

func (*ScalarPathResolver) Supports(parameter ParameterMeta) bool {
	plan, _, err := input.Inspect(parameter.Type)
	return err == nil && plan.Scalar
}

func (*ScalarPathResolver) Resolve(ctx core.ExecutionContext, parameter ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}
	raw, ok := httpCtx.Params()[parameter.PathKey]
	if !ok || parameter.PathKey == "" {
		return nil, &httperr.InputError{Source: "path", Cause: httperr.BadRequest("missing path parameter")}
	}
	value := reflect.New(parameter.Type.Elem())
	if err := input.BindScalarPath(value.Elem(), parameter.PathKey, raw); err != nil {
		return nil, &httperr.InputError{Source: "path", Cause: httperr.BadRequest(err.Error())}
	}
	return value.Convert(parameter.Type).Interface(), nil
}
