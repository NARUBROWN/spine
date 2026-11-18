package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/pkg/query"
)

// QueryValuesResolver preserves value injection for the deprecated query API.
type QueryValuesResolver struct{}

func (*QueryValuesResolver) Supports(pm ParameterMeta) bool {
	return pm.Type == reflect.TypeFor[query.Values]()
}

func (*QueryValuesResolver) Resolve(ctx *execution.Context, _ ParameterMeta) (any, error) {
	httpCtx := ctx.Input.HTTPRequest
	ok := httpCtx != nil
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}
	return query.NewValues(httpCtx.Queries()), nil
}
