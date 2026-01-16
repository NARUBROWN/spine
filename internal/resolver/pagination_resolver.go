package resolver

import (
	"reflect"
	"strconv"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/query"
)

type PaginationResolver struct{}

func (r *PaginationResolver) Supports(parameterMeta ParameterMeta) bool {
	return parameterMeta.Type == reflect.TypeFor[query.Pagination]()
}

func (r *PaginationResolver) Resolve(ctx core.Context, parameterMeta ParameterMeta) (any, error) {
	page := parseInt(ctx.Query("page"), 1)
	size := parseInt(ctx.Query("size"), 20)

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
