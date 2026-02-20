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
	wsCtx, ok := ctx.(core.WebSocketContext)
	if !ok {
		return nil, fmt.Errorf("WebSocketContext가 아닙니다")
	}
	return wsCtx.Payload(), nil
}
