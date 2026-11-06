package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/resolver"
)

type PayloadResolver struct{}

func (r *PayloadResolver) Supports(meta resolver.ParameterMeta) bool {
	return meta.Type.Kind() == reflect.Slice &&
		meta.Type.Elem().Kind() == reflect.Uint8
}

func (r *PayloadResolver) Resolve(ctx core.ExecutionContext, meta resolver.ParameterMeta) (any, error) {
	consumerCtx, ok := ctx.(core.ConsumerRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not a ConsumerRequestContext")
	}

	payload := consumerCtx.Payload()

	value := reflect.ValueOf(payload)
	if !value.Type().ConvertibleTo(meta.Type) {
		return nil, fmt.Errorf("payload type %v is not convertible to %v", value.Type(), meta.Type)
	}
	return value.Convert(meta.Type).Interface(), nil
}
