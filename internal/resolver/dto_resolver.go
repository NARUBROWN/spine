package resolver

import (
	"fmt"
	"reflect"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/input"
)

type DTOResolver struct {
	Validator validation.Validator
}

func (r *DTOResolver) Supports(pm ParameterMeta) bool {
	// 실행 컨텍스트 제외
	if pm.Type == reflect.TypeFor[core.ExecutionContext]() {
		return false
	}

	// 반드시 포인터
	if pm.Type.Kind() != reflect.Ptr {
		return false
	}

	if _, _, ok := input.DTO(pm.Type); ok {
		return false
	}

	elem := pm.Type.Elem()
	if elem.Kind() != reflect.Struct {
		return false
	}

	// form 태그가 하나라도 있으면 FormDTO로 넘긴다
	for i := 0; i < elem.NumField(); i++ {
		if elem.Field(i).Tag.Get("form") != "" {
			return false
		}
	}

	return true
}

func (r *DTOResolver) Resolve(ctx core.ExecutionContext, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}

	// 빈 DTO 생성 (*T)
	valuePtr := reflect.New(parameterMeta.Type.Elem())

	if err := httpCtx.Bind(valuePtr.Interface()); err != nil {
		return nil, fmt.Errorf(
			"DTO binding failed (%s): %w",
			parameterMeta.Type.Name(),
			err,
		)
	}

	// 포인터로 전달
	if err := ValidateDTO(ctx.Context(), r.Validator, valuePtr.Interface()); err != nil {
		return nil, err
	}
	return valuePtr.Interface(), nil
}
