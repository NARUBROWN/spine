package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/query"
)

// QueryValuesResolver는 Deprecated 쿼리 API의 기존 값 주입을 유지합니다.
type QueryValuesResolver struct{}

func (*QueryValuesResolver) Supports(pm ParameterMeta) bool {
	return pm.Type == reflect.TypeFor[query.Values]()
}

func (*QueryValuesResolver) Resolve(ctx core.ExecutionContext, _ ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}
	return query.NewValues(httpCtx.Queries()), nil
}
