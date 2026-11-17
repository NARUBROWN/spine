package resolver

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/resolver"
)

type PayloadResolver struct{}

func (r *PayloadResolver) Supports(meta resolver.ParameterMeta) bool {
	return reflect.TypeFor[[]byte]().ConvertibleTo(meta.Type) && meta.Type.Kind() == reflect.Slice
}

func (r *PayloadResolver) Resolve(ctx execution.Context, meta resolver.ParameterMeta, source *execution.Input) (any, error) {
	message, ok := source.WebSocketMessage()
	if !ok {
		return nil, fmt.Errorf("WebSocket message input is not available")
	}
	payload := reflect.ValueOf(slices.Clone(message.Payload))
	if !payload.Type().ConvertibleTo(meta.Type) {
		return nil, fmt.Errorf("payload type %v is not convertible to %v", payload.Type(), meta.Type)
	}
	return payload.Convert(meta.Type).Interface(), nil
}
