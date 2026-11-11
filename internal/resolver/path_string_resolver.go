package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/pkg/path"
)

type PathStringResolver struct{}

func (r *PathStringResolver) Supports(parameterMeta ParameterMeta) bool {
	return parameterMeta.Type == reflect.TypeFor[path.String]()
}

func (r *PathStringResolver) Resolve(ctx execution.Context, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(execution.HTTPRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}

	if parameterMeta.PathKey == "" {
		return nil, fmt.Errorf(
			"path key is not bound: %v",
			parameterMeta.Type,
		)
	}

	raw, ok := httpCtx.Params()[parameterMeta.PathKey]
	if !ok {
		return nil, fmt.Errorf(
			"path parameter not found: %s",
			parameterMeta.PathKey,
		)
	}

	return path.String{Value: raw}, nil
}
