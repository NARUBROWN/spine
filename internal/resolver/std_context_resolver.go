package resolver

import (
	"context"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/runtime"
)

type StdContextResolver struct{}

func (r *StdContextResolver) Supports(parameterMeta ParameterMeta) bool {
	return parameterMeta.Type == reflect.TypeFor[context.Context]()
}

func (r *StdContextResolver) Resolve(ctx core.ExecutionContext, parameterMeta ParameterMeta) (any, error) {
	return runtime.HandlerGoContext(ctx), nil
}
