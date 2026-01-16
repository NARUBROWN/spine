package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/path"
)

type PathStringResolver struct{}

func (r *PathStringResolver) Supports(parameterMeta ParameterMeta) bool {
	return parameterMeta.Type == reflect.TypeFor[path.String]()
}

func (r *PathStringResolver) Resolve(ctx core.Context, parameterMeta ParameterMeta) (any, error) {
	raw, ok := ctx.Params()[parameterMeta.Type.Name()]
	if !ok {
		return nil, fmt.Errorf("Path param을 찾을 수 없습니다. %s", parameterMeta.Type.Name())
	}
	return path.String{Value: raw}, nil
}
