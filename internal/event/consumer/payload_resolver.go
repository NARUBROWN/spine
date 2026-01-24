package consumer

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

func (r *PayloadResolver) Resolve(ctx core.RequestContext, meta resolver.ParameterMeta) (any, error) {
	consumerCtx, ok := ctx.(core.ConsumerRequestContext)
	if !ok {
		return nil, fmt.Errorf("ConsumerRequestContext가 아닙니다")
	}

	payload := consumerCtx.Payload()
	if payload == nil {
		return nil, fmt.Errorf("Payload를 RequestContext에서 찾을 수 없습니다")
	}

	return payload, nil
}
