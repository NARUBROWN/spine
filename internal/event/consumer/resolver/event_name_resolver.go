package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/resolver"
)

type EventNameResolver struct{}

func (r *EventNameResolver) Supports(meta resolver.ParameterMeta) bool {
	return meta.Type.Kind() == reflect.String
}

func (r *EventNameResolver) Resolve(ctx execution.Context, meta resolver.ParameterMeta) (any, error) {
	consumerCtx, ok := ctx.(execution.ConsumerRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not a ConsumerRequestContext")
	}

	name := consumerCtx.EventName()
	if name == "" {
		return nil, fmt.Errorf("EventName not found in RequestContext")
	}

	value := reflect.ValueOf(name)
	if !value.Type().ConvertibleTo(meta.Type) {
		return nil, fmt.Errorf("event name type %v is not convertible to %v", value.Type(), meta.Type)
	}
	return value.Convert(meta.Type).Interface(), nil
}
