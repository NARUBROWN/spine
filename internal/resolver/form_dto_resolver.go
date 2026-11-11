package resolver

import (
	"fmt"
	"reflect"
	"sync"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/input"
)

type FormDTOResolver struct {
	Validator validation.Validator
	media     sync.Map
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
	return resolveBodyDTO(ctx, parameterMeta, r.Validator, &r.media)
}
