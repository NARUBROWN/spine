package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
)

type DTOResolver struct{}

func (r *DTOResolver) Supports(paramType reflect.Type) bool {
	// Context 제외
	if paramType == reflect.TypeFor[core.Context]() {
		return false
	}

	if paramType.Kind() != reflect.Struct {
		return false
	}

	// query 태그가 하나라도 있으면 QueryDTO가 담당
	for i := 0; i < paramType.NumField(); i++ {
		if paramType.Field(i).Tag.Get("query") != "" {
			return false
		}
	}

	return true
}

func (r *DTOResolver) Resolve(ctx core.Context, paramType reflect.Type) (any, error) {
	// 빈 DTO 생성
	valuePtr := reflect.New(paramType)

	if err := ctx.Bind(valuePtr.Interface()); err != nil {
		return nil, fmt.Errorf(
			"DTO 바인딩 실패 (%s): %w",
			paramType.Name(),
			err,
		)
	}

	// 포인터가 아니라 값으로 전달
	return valuePtr.Elem().Interface(), nil
}
