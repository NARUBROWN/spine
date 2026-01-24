package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
)

type FormDTOResolver struct{}

func (r *FormDTOResolver) Supports(pm ParameterMeta) bool {
	if pm.Type.Kind() != reflect.Ptr {
		return false
	}

	elem := pm.Type.Elem()
	if elem.Kind() != reflect.Struct {
		return false
	}

	for i := 0; i < elem.NumField(); i++ {
		if elem.Field(i).Tag.Get("form") != "" {
			return true
		}
	}

	return false
}

func (r *FormDTOResolver) Resolve(ctx core.RequestContext, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("HTTP 요청 컨텍스트가 아닙니다")
	}

	dto := reflect.New(parameterMeta.Type.Elem()).Interface()

	// Echo의 Form 바인딩 위임
	if err := httpCtx.Bind(dto); err != nil {
		return nil, err
	}

	return dto, nil
}
