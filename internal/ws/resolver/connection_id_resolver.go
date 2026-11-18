package resolver

import (
	"fmt"
	"reflect"

	"context"
	"github.com/NARUBROWN/spine/internal/resolver"
	pkgws "github.com/NARUBROWN/spine/pkg/ws"
)

type ConnectionIDResolver struct{}

func (r *ConnectionIDResolver) Supports(meta resolver.ParameterMeta) bool {
	return meta.Type == reflect.TypeFor[pkgws.ConnectionID]()
}

func (r *ConnectionIDResolver) Resolve(ctx context.Context, meta resolver.ParameterMeta, source *Input) (any, error) {
	id := ""
	if source != nil {
		id = source.ConnectionID
	}
	if id == "" {
		return nil, fmt.Errorf("WebSocket connection ID is not available")
	}
	return pkgws.ConnectionID{
		Value: id,
	}, nil
}
