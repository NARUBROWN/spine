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

func (r *PathIntResolver) Resolve(ctx core.RequestContext, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("HTTP 요청 컨텍스트가 아닙니다")
	}

	if parameterMeta.PathKey == "" {
		return nil, fmt.Errorf("%v에 해당하는 path key가 매칭되지 않았습니다.", parameterMeta.Type)
	}
	raw, ok := httpCtx.Params()[parameterMeta.PathKey]
	if !ok {
		return nil, fmt.Errorf("path param을 찾을 수 없습니다. %s", parameterMeta.PathKey)
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf(
			"유효하지 않은 Path param %s: %v",
			parameterMeta.Type.Name(),
			err,
		)
	}

	return path.Int{Value: value}, nil
}
