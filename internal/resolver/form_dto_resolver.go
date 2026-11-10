package resolver

import (
	"fmt"
	"github.com/NARUBROWN/spine/internal/inputerr"
	"reflect"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/input"
)

type FormDTOResolver struct {
	Validator validation.Validator
}

func (r *FormDTOResolver) Supports(pm ParameterMeta) bool {
	source, wrapped := input.Wrapper(pm.Type)
	if !wrapped || source != "body" || pm.Type.Kind() != reflect.Pointer || pm.Type.Elem().Kind() != reflect.Struct {
		return false
	}
	return input.HasForm(pm.Type.Elem())
}

func (r *FormDTOResolver) Resolve(ctx execution.Context, parameterMeta ParameterMeta) (any, error) {
	if !r.Supports(parameterMeta) {
		return nil, fmt.Errorf("HTTP DTO inputs require an explicit input wrapper")
	}
	httpCtx, ok := ctx.(execution.HTTPRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}

	dto := reflect.New(parameterMeta.Type.Elem()).Interface()

	// 요청별 공통 본문 파서에서 폼 값을 읽습니다.
	if err := httpCtx.Bind(dto); err != nil {
		return nil, &inputerr.Error{Source: "body", Cause: err}
	}

	if err := ValidateHTTPDTO(ctx.Context(), r.Validator, dto, "body", bodyFieldTag(httpCtx)); err != nil {
		return nil, err
	}
	return reflect.ValueOf(dto).Convert(parameterMeta.Type).Interface(), nil
}
