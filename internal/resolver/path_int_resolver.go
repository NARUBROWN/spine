package resolver

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/path"
)

type PathIntResolver struct{}

func (r *PathIntResolver) Supports(parameterMeta ParameterMeta) bool {
	return parameterMeta.Type == reflect.TypeFor[path.Int]()
}

func (r *PathIntResolver) Resolve(ctx core.ExecutionContext, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}

	if parameterMeta.PathKey == "" {
		return nil, fmt.Errorf("no path key matches %v", parameterMeta.Type)
	}
	raw, ok := httpCtx.Params()[parameterMeta.PathKey]
	if !ok {
		return nil, fmt.Errorf("path parameter not found: %s", parameterMeta.PathKey)
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid path parameter %s: %v",
			parameterMeta.Type.Name(),
			err,
		)
	}

	return path.Int{Value: value}, nil
}
