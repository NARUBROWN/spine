package resolver

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/query"
)

type PaginationResolver struct{}

func (r *PaginationResolver) Supports(parameterMeta ParameterMeta) bool {
	return parameterMeta.Type == reflect.TypeFor[query.Pagination]()
}

func (r *PaginationResolver) Resolve(ctx core.RequestContext, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("HTTP 요청 컨텍스트가 아닙니다")
	}

	page := parseInt(httpCtx.Query("page"), 1)
	size := parseInt(httpCtx.Query("size"), 20)

	return query.Pagination{
		Page: page,
		Size: size,
	}, nil
}

func parseInt(value string, defaultValue int) int {
	result, err := strconv.Atoi(value)
	if err != nil || value == "" {
		return defaultValue
	}
	return result
}
