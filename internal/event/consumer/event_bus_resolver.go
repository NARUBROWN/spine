package consumer

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/event/publish"
	"github.com/NARUBROWN/spine/internal/resolver"
)

type EventBusResolver struct{}

func (r *EventBusResolver) Supports(parameterMeta resolver.ParameterMeta) bool {
	eventBusType := reflect.TypeOf((*publish.EventBus)(nil)).Elem()
	return parameterMeta.Type == eventBusType
}

func (r *EventBusResolver) Resolve(ctx core.RequestContext, meta resolver.ParameterMeta) (any, error) {

	bus := ctx.EventBus()
	if bus == nil {
		return nil, fmt.Errorf("EventBus가 RequestContext에 존재하지 않습니다")
	}

	return bus, nil
}
