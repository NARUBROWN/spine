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
		var declaration validation.SchemaDescription
		if describer, ok := validator.(validation.SchemaDescriber); ok && input.BodyMediaNeedsValidation(payload) {
			declaration, _ = DescribeValidation(describer, payload)
		}
		available, _ = mediaCache.LoadOrStore(payload, input.BodyMediaTypes(payload, declaration))
	}
	mediaTypes := available.([]string)
	media, _, _ := mime.ParseMediaType(httpCtx.Header("Content-Type"))
	switch media {
	case "application/json", "application/xml", "text/xml", "application/x-www-form-urlencoded", "multipart/form-data":
		if !slices.Contains(mediaTypes, media) {
			return nil, &inputerr.Error{Source: "body", Cause: &httpfailure.HTTPError{
				Status: http.StatusUnsupportedMediaType, Message: "Unsupported Media Type",
			}}
		}
	case "":
		if len(mediaTypes) > 0 {
			media = mediaTypes[0]
		}
	}
	dto := reflect.New(payload)
	if err := httpCtx.Bind(dto.Interface()); err != nil {
		return nil, &inputerr.Error{Source: "body", Cause: err}
	}
	if err := ValidateHTTPDTO(ctx.Context(), validator, dto.Interface(), "body", input.BodyFieldTag(media)); err != nil {
		return nil, err
	}
	return dto.Convert(parameter.Type).Interface(), nil
}
