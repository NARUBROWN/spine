package resolver

import (
	"context"
	"reflect"

	"github.com/NARUBROWN/spine/internal/execution"
)

type StdContextResolver struct{}

func (r *StdContextResolver) Supports(parameterMeta ParameterMeta) bool {
	return parameterMeta.Type == reflect.TypeFor[context.Context]()
}

func (r *StdContextResolver) Resolve(ctx execution.Context, parameterMeta ParameterMeta) (any, error) {
	return execution.GoContext(ctx), nil
}
