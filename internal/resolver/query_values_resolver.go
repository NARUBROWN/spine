package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/query"
)

type QueryValuesResolver struct{}

func (r *QueryValuesResolver) Supports(pm ParameterMeta) bool {
	return pm.Type == reflect.TypeFor[query.Values]()
}

func (r *QueryValuesResolver) Resolve(ctx core.RequestContext, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("HTTP 요청 컨텍스트가 아닙니다")
	}
	return query.NewValues(httpCtx.Queries()), nil
}
