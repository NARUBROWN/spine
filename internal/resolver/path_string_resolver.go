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

func (r *PathStringResolver) Resolve(ctx core.RequestContext, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("HTTP 요청 컨텍스트가 아닙니다")
	}

	if parameterMeta.PathKey == "" {
		return nil, fmt.Errorf(
			"path key가 바인딩되지 않았습니다: %v",
			parameterMeta.Type,
		)
	}

	raw, ok := httpCtx.Params()[parameterMeta.PathKey]
	if !ok {
		return nil, fmt.Errorf(
			"path param을 찾을 수 없습니다: %s",
			parameterMeta.PathKey,
		)
	}

	return path.String{Value: raw}, nil
}
