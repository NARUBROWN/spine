package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/resolver"
	pkgws "github.com/NARUBROWN/spine/pkg/ws"
)

type ConnectionIDResolver struct{}

func (r *ConnectionIDResolver) Supports(meta resolver.ParameterMeta) bool {
	return meta.Type == reflect.TypeFor[pkgws.ConnectionID]()
}

func (r *ConnectionIDResolver) Resolve(ctx execution.Context, meta resolver.ParameterMeta) (any, error) {
	wsCtx, ok := ctx.(execution.WebSocketContext)
	if !ok {
		return nil, fmt.Errorf("context is not a WebSocketContext")
	}
	return pkgws.ConnectionID{
		Value: wsCtx.ConnID(),
	}, nil
}
