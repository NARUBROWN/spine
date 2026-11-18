package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/spine"
)

type EventNameResolver struct{}

func (r *EventNameResolver) Supports(meta resolver.ParameterMeta) bool {
	return meta.Type.Kind() == reflect.String
}

func (r *EventNameResolver) Resolve(ctx spine.EventCtx, meta resolver.ParameterMeta, message *Input) (any, error) {
	if message == nil {
		return nil, fmt.Errorf("consumer message input is not available")
	}

	name := message.Name
	if name == "" {
		return nil, fmt.Errorf("event name is empty")
	}

	value := reflect.ValueOf(name)
	if !value.Type().ConvertibleTo(meta.Type) {
		return nil, fmt.Errorf("event name type %v is not convertible to %v", value.Type(), meta.Type)
	}
	return value.Convert(meta.Type).Interface(), nil
}
