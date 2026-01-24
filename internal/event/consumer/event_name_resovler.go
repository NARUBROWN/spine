package consumer

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

func (r *EventNameResolver) Resolve(ctx core.RequestContext, meta resolver.ParameterMeta) (any, error) {
	consumerCtx, ok := ctx.(core.ConsumerRequestContext)
	if !ok {
		return nil, fmt.Errorf("ConsumerRequestContext가 아닙니다")
	}

	name := consumerCtx.EventName()
	if name == "" {
		return nil, fmt.Errorf("EventName을 RequestContext에서 찾을 수 없습니다")
	}

	return name, nil
}
