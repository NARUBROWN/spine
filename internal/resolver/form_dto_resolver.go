package resolver

import (
	"fmt"
	"reflect"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/input"
	"github.com/NARUBROWN/spine/pkg/httperr"
)

type FormDTOResolver struct {
	Validator validation.Validator
}

func (r *FormDTOResolver) Supports(pm ParameterMeta) bool {
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

	return input.HasForm(elem)
}

func (r *FormDTOResolver) Resolve(ctx core.ExecutionContext, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}

	dto := reflect.New(parameterMeta.Type.Elem()).Interface()

	// 요청별 공통 본문 파서에서 폼 값을 읽습니다.
	if err := httpCtx.Bind(dto); err != nil {
		return nil, &httperr.InputError{Source: "body", Cause: err}
	}

	if err := ValidateHTTPDTO(ctx.Context(), r.Validator, dto, "body", bodyFieldTag(httpCtx)); err != nil {
		return nil, err
	}
	return dto, nil
}
