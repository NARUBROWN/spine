package resolver

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/spine"
)

type PayloadResolver struct{}

func (r *PayloadResolver) Supports(meta resolver.ParameterMeta) bool {
	return reflect.TypeFor[[]byte]().ConvertibleTo(meta.Type) && meta.Type.Kind() == reflect.Slice
}

func (r *PayloadResolver) Resolve(ctx spine.EventCtx, meta resolver.ParameterMeta, message *Input) (any, error) {
	if message == nil {
		return nil, fmt.Errorf("consumer message input is not available")
	}

	payload := slices.Clone(message.Payload)

	value := reflect.ValueOf(payload)
	if !value.Type().ConvertibleTo(meta.Type) {
		return nil, fmt.Errorf("payload type %v is not convertible to %v", value.Type(), meta.Type)
	}
	return value.Convert(meta.Type).Interface(), nil
}
