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

func (r *PayloadResolver) Resolve(ctx execution.Context, meta resolver.ParameterMeta) (any, error) {
	consumerCtx, ok := ctx.(execution.ConsumerRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not a ConsumerRequestContext")
	}

	payload := slices.Clone(consumerCtx.Payload())

	value := reflect.ValueOf(payload)
	if !value.Type().ConvertibleTo(meta.Type) {
		return nil, fmt.Errorf("payload type %v is not convertible to %v", value.Type(), meta.Type)
	}
	return value.Convert(meta.Type).Interface(), nil
}
