package resolver

import (
	"fmt"
	"net/http"
	"reflect"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/input"
	"github.com/NARUBROWN/spine/pkg/httperr"
)

type SourceDTOResolver struct{ Validator validation.Validator }

func (*SourceDTOResolver) Supports(parameter ParameterMeta) bool {
	_, _, ok := input.DTO(parameter.Type)
	return ok
}

func (r *SourceDTOResolver) Resolve(ctx core.ExecutionContext, parameter ParameterMeta) (any, error) {
	t, source, _ := input.DTO(parameter.Type)
	fields, err := input.Fields(t, source)
	if err != nil {
		return nil, err
	}
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}
	values := httpCtx.Queries()
	if source == "header" {
		values = map[string][]string{}
		for name, list := range httpCtx.Headers() {
			key := http.CanonicalHeaderKey(name)
			values[key] = append(values[key], list...)
		}
	}
	value := reflect.New(t)
	if err := input.Bind(value.Elem(), fields, values, source); err != nil {
		return nil, httperr.BadRequest(err.Error())
	}
	if err := ValidateDTO(ctx.Context(), r.Validator, value.Interface()); err != nil {
		return nil, err
	}
	wrapperType := parameter.Type
	if wrapperType.Kind() == reflect.Pointer {
		wrapperType = wrapperType.Elem()
	}
	wrapper := reflect.New(wrapperType)
	wrapper.Elem().Field(0).Set(value.Elem())
	if parameter.Type.Kind() == reflect.Pointer {
		return wrapper.Interface(), nil
	}
	return wrapper.Elem().Interface(), nil
}
