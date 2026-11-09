package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/header"
)

// HeaderResolver는 Deprecated 헤더 API의 기존 값 주입을 유지합니다.
type HeaderResolver struct{}

func (*HeaderResolver) Supports(pm ParameterMeta) bool {
	return pm.Type == reflect.TypeFor[header.Values]()
}

func (*HeaderResolver) Resolve(ctx core.ExecutionContext, _ ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}
	return header.NewValues(httpCtx.Headers()), nil
}
