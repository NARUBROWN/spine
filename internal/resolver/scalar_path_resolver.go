package resolver

import (
	"fmt"
	"github.com/NARUBROWN/spine/internal/inputerr"
	"reflect"

	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/httpfailure"
	"github.com/NARUBROWN/spine/internal/input"
)

type ScalarPathResolver struct{}

func (*ScalarPathResolver) Supports(parameter ParameterMeta) bool {
	plan, _, err := input.Inspect(parameter.Type)
	return err == nil && plan.Scalar
}

func (*ScalarPathResolver) Resolve(ctx execution.Context, parameter ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(execution.HTTPRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}
	raw, ok := httpCtx.Params()[parameter.PathKey]
	if !ok || parameter.PathKey == "" {
		return nil, &inputerr.Error{Source: "path", Cause: httpfailure.BadRequest("missing path parameter")}
	}
	value := reflect.New(parameter.Type.Elem())
	if err := input.BindScalarPath(value.Elem(), parameter.PathKey, raw); err != nil {
		return nil, &inputerr.Error{Source: "path", Cause: httpfailure.BadRequest(err.Error())}
	}
	return value.Convert(parameter.Type).Interface(), nil
}
