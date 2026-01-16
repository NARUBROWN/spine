package resolver

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/NARUBROWN/spine/core"
)

type PrimitiveResolver struct{}

func (r *PrimitiveResolver) Supports(parameterMeta ParameterMeta) bool {
	return parameterMeta.Type.Kind() == reflect.String || parameterMeta.Type.Kind() == reflect.Int
}

func (r *PrimitiveResolver) Resolve(ctx core.Context, parameterMeta ParameterMeta) (any, error) {

	var raw string

	// PathParam 우선
	pathParams := ctx.Params()
	if len(pathParams) == 1 {
		for _, value := range pathParams {
			raw = value
		}
	}

	if raw == "" {
		queryParams := ctx.Queries()
		if len(queryParams) == 1 {
			for _, value := range queryParams {
				if len(value) > 0 {
					raw = value[0]
				}
			}
		}
	}

	if raw == "" {
		return nil, fmt.Errorf(
			"primitive 파라미터를 자동 매핑할 수 없습니다. (path=%d, query=%d)",
			len(pathParams),
			len(ctx.Queries()),
		)
	}

	// 타입 변환
	switch parameterMeta.Type.Kind() {
	case reflect.String:
		return raw, nil
	case reflect.Int:
		value, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("int 변환 실패 : %w", err)
		}
		return value, nil
	}
	panic("도달할 수 없는 조건")
}
