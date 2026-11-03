package resolver

import (
	"fmt"
	"reflect"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/core"
)

type FormDTOResolver struct {
	Validator validation.Validator
}

func (r *FormDTOResolver) Supports(pm ParameterMeta) bool {
	if pm.Type.Kind() != reflect.Ptr {
		return false
	}

	elem := pm.Type.Elem()
	if elem.Kind() != reflect.Struct {
		return false
	}

	for i := 0; i < elem.NumField(); i++ {
		if elem.Field(i).Tag.Get("form") != "" {
			return true
		}
	}

	return false
}

func (r *FormDTOResolver) Resolve(ctx core.ExecutionContext, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}

	dto := reflect.New(parameterMeta.Type.Elem()).Interface()

	// Echo의 폼 바인딩에 위임
	if err := httpCtx.Bind(dto); err != nil {
		return nil, err
	}

	if err := ValidateDTO(ctx.Context(), r.Validator, dto); err != nil {
		return nil, err
	}
	return dto, nil
}
