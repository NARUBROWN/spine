package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/pkg/header"
)

// HeaderResolver는 Deprecated 헤더 API의 기존 값 주입을 유지합니다.
type HeaderResolver struct{}

func (*HeaderResolver) Supports(pm ParameterMeta) bool {
	return pm.Type == reflect.TypeFor[header.Values]()
}

func (*HeaderResolver) Resolve(ctx execution.Context, _ ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(execution.HTTPRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}
	return header.NewValues(httpCtx.Headers()), nil
}
