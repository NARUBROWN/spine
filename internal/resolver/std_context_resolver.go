package resolver

import (
	"context"
	"reflect"

	"github.com/NARUBROWN/spine/core"
)

type StdContextResolver struct{}

func (r *StdContextResolver) Supports(parameterMeta ParameterMeta) bool {
	return parameterMeta.Type == reflect.TypeFor[context.Context]()
}

func (r *StdContextResolver) Resolve(ctx core.RequestContext, parameterMeta ParameterMeta) (any, error) {
	return ctx.Context(), nil
}
