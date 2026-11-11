package resolver

import (
	"fmt"
	"github.com/NARUBROWN/spine/internal/inputerr"
	"reflect"
	"strconv"

	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/httpfailure"
	"github.com/NARUBROWN/spine/pkg/query"
)

// PaginationResolver는 Deprecated 페이지네이션 입력의 기존 파싱을 유지합니다.
type PaginationResolver struct{}

func (r *PaginationResolver) Supports(parameterMeta ParameterMeta) bool {
	return parameterMeta.Type == reflect.TypeFor[query.Pagination]()
}

func (r *PaginationResolver) Resolve(ctx execution.Context, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(execution.HTTPRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}
	values := httpCtx.Queries()
	for _, name := range []string{"page", "size"} {
		if len(values[name]) > 1 {
			return nil, &inputerr.Error{Source: "query", Cause: httpfailure.BadRequest("multiple values for query field " + name)}
		}
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
