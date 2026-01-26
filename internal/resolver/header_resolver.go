package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/header"
)

type HeaderResolver struct{}

func (hr *HeaderResolver) Supports(pm ParameterMeta) bool {
	return pm.Type == reflect.TypeFor[header.Values]()
}

func (hr *HeaderResolver) Resolve(ctx core.ExecutionContext, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("HTTP 요청 컨텍스트가 아닙니다")
	}
	return header.NewValues(httpCtx.Headers()), nil
}
