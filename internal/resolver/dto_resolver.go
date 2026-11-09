package resolver

import (
	"fmt"
	"mime"
	"reflect"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/input"
	"github.com/NARUBROWN/spine/pkg/httperr"
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

	return !input.HasForm(elem)
}

func (r *DTOResolver) Resolve(ctx core.ExecutionContext, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}

	// 빈 DTO 생성 (*T)
	valuePtr := reflect.New(parameterMeta.Type.Elem())

	if err := httpCtx.Bind(valuePtr.Interface()); err != nil {
		return nil, &httperr.InputError{Source: "body", Cause: fmt.Errorf(
			"DTO binding failed (%s): %w",
			parameterMeta.Type.Name(),
			err,
		)}
	}

	// 포인터로 전달
	if err := ValidateHTTPDTO(ctx.Context(), r.Validator, valuePtr.Interface(), "body", bodyFieldTag(httpCtx)); err != nil {
		return nil, err
	}
	return valuePtr.Interface(), nil
}

func bodyFieldTag(ctx core.HttpRequestContext) string {
	media, _, _ := mime.ParseMediaType(ctx.Header("Content-Type"))
	if media == "multipart/form-data" || media == "application/x-www-form-urlencoded" {
		return "form"
	}
	if media == "application/xml" || media == "text/xml" {
		return "xml"
	}
	return "json"
}
