package resolver

import (
	"fmt"
	"net/http"
	"reflect"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/input"
	"github.com/NARUBROWN/spine/pkg/httperr"
)

type InputDTOResolver struct{ Validator validation.Validator }

func (*InputDTOResolver) Supports(parameter ParameterMeta) bool {
	_, _, ok := input.DTO(parameter.Type)
	return ok && parameter.Type.Kind() == reflect.Pointer
}

func (r *InputDTOResolver) Resolve(ctx core.ExecutionContext, parameter ParameterMeta) (any, error) {
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
	var bindingErr error
	if raw, ok := httpCtx.(httpcontract.RawQueryContext); source == "query" && ok {
		if query, available := raw.RawQuery(); available {
			bindingErr = input.BindQuery(value.Elem(), fields, query)
		} else {
			bindingErr = input.Bind(value.Elem(), fields, values, source)
		}
	} else {
		bindingErr = input.Bind(value.Elem(), fields, values, source)
	}
	if bindingErr != nil {
		return nil, httperr.BadRequest(bindingErr.Error())
	}
	if err := ValidateDTO(ctx.Context(), r.Validator, value.Interface()); err != nil {
		return nil, err
	}
	return value.Interface(), nil
}
