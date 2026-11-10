package resolver

import (
	"fmt"
	"github.com/NARUBROWN/spine/internal/inputerr"
	"mime"
	"reflect"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/input"
)

type DTOResolver struct {
	Validator validation.Validator
}

func (r *DTOResolver) Supports(pm ParameterMeta) bool {
	source, wrapped := input.Wrapper(pm.Type)
	if !wrapped || source != "body" || pm.Type.Kind() != reflect.Pointer || pm.Type.Elem().Kind() != reflect.Struct {
		return false
	}
	return !input.HasForm(pm.Type.Elem())
}

func (r *DTOResolver) Resolve(ctx execution.Context, parameterMeta ParameterMeta) (any, error) {
	if !r.Supports(parameterMeta) {
		return nil, fmt.Errorf("HTTP DTO inputs require an explicit input wrapper")
	}
	httpCtx, ok := ctx.(execution.HTTPRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}

	// 빈 DTO 생성 (*T)
	valuePtr := reflect.New(parameterMeta.Type.Elem())

	if err := httpCtx.Bind(valuePtr.Interface()); err != nil {
		return nil, &inputerr.Error{Source: "body", Cause: fmt.Errorf(
			"DTO binding failed (%s): %w",
			parameterMeta.Type.Name(),
			err,
		)}
	}

	// 포인터로 전달
	if err := ValidateHTTPDTO(ctx.Context(), r.Validator, valuePtr.Interface(), "body", bodyFieldTag(httpCtx)); err != nil {
		return nil, err
	}
	return valuePtr.Convert(parameterMeta.Type).Interface(), nil
}

func bodyFieldTag(ctx execution.HTTPRequestContext) string {
	media, _, _ := mime.ParseMediaType(ctx.Header("Content-Type"))
	if media == "multipart/form-data" || media == "application/x-www-form-urlencoded" {
		return "form"
	}
	if media == "application/xml" || media == "text/xml" {
		return "xml"
	}
	return "json"
}
