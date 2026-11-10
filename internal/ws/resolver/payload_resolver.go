package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/resolver"
)

type PayloadResolver struct{}

func (r *PayloadResolver) Supports(meta resolver.ParameterMeta) bool {
	return meta.Type.Kind() == reflect.Slice &&
		meta.Type.Elem().Kind() == reflect.Uint8
}

func (r *PayloadResolver) Resolve(ctx execution.Context, meta resolver.ParameterMeta) (any, error) {
	wsCtx, ok := ctx.(execution.WebSocketContext)
	if !ok {
		return nil, fmt.Errorf("context is not a WebSocketContext")
	}
	return wsCtx.Payload(), nil
}
