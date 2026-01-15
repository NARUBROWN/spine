package resolver

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/NARUBROWN/spine/core"
)

type QueryDTOResolver struct{}

func (r *QueryDTOResolver) Supports(paramType reflect.Type) bool {
	if paramType.Kind() != reflect.Struct {
		return false
	}

	for i := 0; i < paramType.NumField(); i++ {
		if tag := paramType.Field(i).Tag.Get("query"); tag != "" {
			return true
		}
	}

	return false
}

func (r *QueryDTOResolver) Resolve(ctx core.Context, paramType reflect.Type) (any, error) {
	dto := reflect.New(paramType).Elem()

	for i := 0; i < paramType.NumField(); i++ {
		field := paramType.Field(i)
		tag := field.Tag.Get("query")

		if tag == "" {
			continue
		}

		raw := ctx.Query(tag)
		if raw == "" {
			continue
		}

		value, err := convertPrimitive(raw, field.Type)
		if err != nil {
			return nil, fmt.Errorf(
				"QueryDTO 바인딩 실패 (%s.%s): %w",
				paramType.Name(),
				field.Name,
				err,
			)
		}
		dto.Field(i).Set(reflect.ValueOf(value))
	}

	return dto.Interface(), nil
}

func convertPrimitive(raw string, fieldType reflect.Type) (any, error) {
	switch fieldType.Kind() {
	case reflect.String:
		return raw, nil
	case reflect.Int:
		i, err := strconv.Atoi(raw)
		if err != nil {
			return nil, err
		}
		return i, nil
	default:
		return nil, fmt.Errorf("지원하지 않는 타입: %v", fieldType)
	}
}
