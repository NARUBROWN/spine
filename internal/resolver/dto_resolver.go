package resolver

import (
	"fmt"
	"mime"
	"reflect"
	"sync"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/httpfailure"
	"github.com/NARUBROWN/spine/internal/input"
	"github.com/NARUBROWN/spine/internal/inputerr"
)

type DTOResolver struct {
	Validator validation.Validator
	media     sync.Map
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
	return resolveBodyDTO(ctx, parameterMeta, r.Validator, &r.media)
}

func resolveBodyDTO(ctx execution.Context, parameter ParameterMeta, validator validation.Validator, mediaCache *sync.Map) (any, error) {
	httpCtx, ok := ctx.(execution.HTTPRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}
	payload := parameter.Type.Elem()
	available, cached := mediaCache.Load(payload)
	if !cached {
		var plan bodyMediaPlan
		if describer, ok := validator.(validation.SchemaDescriber); ok && input.BodyMediaNeedsValidation(payload) {
			plan.validation, _ = DescribeValidation(describer, payload)
		}
		plan.mediaTypes = input.BodyMediaTypes(payload, plan.validation)
		available, _ = mediaCache.LoadOrStore(payload, plan)
	}
	plan := available.(bodyMediaPlan)
	dto := reflect.New(payload)
	if err := httpCtx.Bind(dto.Interface(), plan.mediaTypes); err != nil {
		return nil, &inputerr.Error{Source: "body", Cause: err}
	}
	media, _, _ := mime.ParseMediaType(httpCtx.Header("Content-Type"))
	if media == "" && len(plan.mediaTypes) > 0 {
		media = plan.mediaTypes[0]
	}
	if input.ActiveUnbindableBodyField(dto.Elem(), media, plan.validation) {
		return nil, &inputerr.Error{Source: "body", Cause: httpfailure.BadRequest("The request body format cannot supply a required field in the provided object")}
	}
	if err := ValidateHTTPDTO(ctx.Context(), validator, dto.Interface(), "body", input.BodyFieldTag(media)); err != nil {
		return nil, err
	}
	return dto.Convert(parameter.Type).Interface(), nil
}

type bodyMediaPlan struct {
	mediaTypes []string
	validation validation.SchemaDescription
}
