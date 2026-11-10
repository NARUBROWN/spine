package resolver

import (
	"fmt"
	"github.com/NARUBROWN/spine/internal/inputerr"
	"net/http"
	"reflect"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/httpfailure"
	"github.com/NARUBROWN/spine/internal/input"
)

type InputDTOResolver struct{ Validator validation.Validator }

func (*InputDTOResolver) Supports(parameter ParameterMeta) bool {
	_, _, ok := input.DTO(parameter.Type)
	return ok && parameter.Type.Kind() == reflect.Pointer
}

func (r *InputDTOResolver) Resolve(ctx execution.Context, parameter ParameterMeta) (any, error) {
	if !r.Supports(parameter) {
		return nil, fmt.Errorf("HTTP DTO inputs require an explicit input wrapper")
	}
	plan, supported, err := input.Inspect(parameter.Type)
	if !supported {
		return nil, fmt.Errorf("use input.Path, input.Query or input.Header for HTTP DTO inputs")
	}
	t, source, fields := plan.Payload, plan.Source, plan.Fields
	if err != nil {
		return nil, err
	}
	httpCtx, ok := ctx.(execution.HTTPRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}
	values := httpCtx.Queries()
	if source == "path" {
		values = map[string][]string{}
		for name, value := range httpCtx.Params() {
			values[name] = []string{value}
		}
	} else if source == "header" {
		values = map[string][]string{}
		for name, list := range httpCtx.Headers() {
			key := http.CanonicalHeaderKey(name)
			values[key] = append(values[key], list...)
		}
	}
	value := reflect.New(t)
	var bindingErr error
	if raw, ok := httpCtx.(httpcontract.RawQueryContext); source == "query" && ok {
		bindingErr = input.BindQuery(value.Elem(), fields, raw.RawQuery())
	} else {
		bindingErr = input.Bind(value.Elem(), fields, values, source)
	}
	if bindingErr != nil {
		return nil, &inputerr.Error{Source: source, Cause: httpfailure.BadRequest(bindingErr.Error())}
	}
	validationContext := ctx.Context()
	if plan.Wrapped {
		validationContext = validation.WithFieldNameMapper(validationContext, plan.FieldName)
	}
	if err := ValidateHTTPDTO(validationContext, r.Validator, value.Interface(), source, source); err != nil {
		return nil, err
	}
	return value.Convert(parameter.Type).Interface(), nil
}
