package resolver

import (
	"fmt"
	"mime"
	"net/http"
	"reflect"
	"slices"
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
		plan.restricted = !slices.Equal(plan.mediaTypes, input.BodyMediaTypes(payload, validation.SchemaDescription{}))
		available, _ = mediaCache.LoadOrStore(payload, plan)
	}
	plan := available.(bodyMediaPlan)
	// 필수 필드 때문에 형식이 제한된 DTO는 빈 본문에서도 해당 계약을 지킵니다.
	// 일반 DTO의 빈 본문 여부와 기본 형식 검사는 공통 바인더가 판정합니다.
	if contentType := httpCtx.Header("Content-Type"); plan.restricted && contentType != "" {
		media, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return nil, &inputerr.Error{Source: "body", Cause: httpfailure.BadRequest("Invalid Content-Type")}
		}
		if !slices.Contains(plan.mediaTypes, media) {
			return nil, &inputerr.Error{Source: "body", Cause: &httpfailure.HTTPError{
				Status: http.StatusUnsupportedMediaType, Message: "Unsupported Media Type",
			}}
		}
	}
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
	restricted bool
}
