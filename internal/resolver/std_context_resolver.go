package resolver

import (
	"context"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/event/publish"
)

type StdContextResolver struct{}

func (r *StdContextResolver) Supports(parameterMeta ParameterMeta) bool {
	return parameterMeta.Type == reflect.TypeFor[context.Context]()
}

func (r *StdContextResolver) Resolve(ctx core.RequestContext, parameterMeta ParameterMeta) (any, error) {
	baseCtx := ctx.Context()
	bus := ctx.EventBus()
	if bus != nil {
		return context.WithValue(baseCtx, publish.PublisherKey, bus), nil
	}
	return baseCtx, nil
}
