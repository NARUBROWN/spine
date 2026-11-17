package resolver

import (
	"fmt"
	"reflect"
	"slices"

	"context"
	"github.com/NARUBROWN/spine/internal/resolver"
)

type PayloadResolver struct{}

func (r *PayloadResolver) Supports(meta resolver.ParameterMeta) bool {
	return reflect.TypeFor[[]byte]().ConvertibleTo(meta.Type) && meta.Type.Kind() == reflect.Slice
}

func (r *PayloadResolver) Resolve(ctx context.Context, meta resolver.ParameterMeta, source *Input) (any, error) {
	message := source
	if message == nil {
		return nil, fmt.Errorf("WebSocket message input is not available")
	}
	payload := reflect.ValueOf(slices.Clone(message.Payload))
	if !payload.Type().ConvertibleTo(meta.Type) {
		return nil, fmt.Errorf("payload type %v is not convertible to %v", payload.Type(), meta.Type)
	}
	return payload.Convert(meta.Type).Interface(), nil
}
