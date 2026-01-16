package resolver

import (
	"reflect"

	"github.com/NARUBROWN/spine/core"
)

// ContextResolver는 spine.Context 타입의 파라미터를 처리합니다.
type ContextResolver struct{}

func (r *ContextResolver) Supports(parameterMeta ParameterMeta) bool {
	// 정확히 spine.Context 타입만 처리
	return parameterMeta.Type == reflect.TypeFor[core.ExecutionContext]()
}

func (r *ContextResolver) Resolve(ctx core.RequestContext, parameterMeta ParameterMeta) (any, error) {
	return ctx, nil
}
