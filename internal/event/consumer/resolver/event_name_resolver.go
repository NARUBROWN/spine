package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/resolver"
)

type EventNameResolver struct{}

func (r *EventNameResolver) Supports(meta resolver.ParameterMeta) bool {
	return meta.Type.Kind() == reflect.String
}

func (r *EventNameResolver) Resolve(ctx core.ExecutionContext, meta resolver.ParameterMeta) (any, error) {
	consumerCtx, ok := ctx.(core.ConsumerRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not a ConsumerRequestContext")
	}

	name := consumerCtx.EventName()
	if name == "" {
		return nil, fmt.Errorf("EventName not found in RequestContext")
	}

	return name, nil
}
