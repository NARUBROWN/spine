package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
)

type DTOResolver struct{}

func (r *DTOResolver) Supports(parameterMeta ParameterMeta) bool {
	// Context 제외
	if parameterMeta.Type == reflect.TypeFor[core.Context]() {
		return false
	}

	if parameterMeta.Type.Kind() != reflect.Struct {
		return false
	}

	// query 태그가 하나라도 있으면 QueryDTO가 담당
	for i := 0; i < parameterMeta.Type.NumField(); i++ {
		if parameterMeta.Type.Field(i).Tag.Get("query") != "" {
			return false
		}
	}

	return true
}

func (r *DTOResolver) Resolve(ctx core.Context, parameterMeta ParameterMeta) (any, error) {
	// 빈 DTO 생성
	valuePtr := reflect.New(parameterMeta.Type)

	if err := ctx.Bind(valuePtr.Interface()); err != nil {
		return nil, fmt.Errorf(
			"DTO 바인딩 실패 (%s): %w",
			parameterMeta.Type.Name(),
			err,
		)
	}

	// 포인터가 아니라 값으로 전달
	return valuePtr.Elem().Interface(), nil
}
