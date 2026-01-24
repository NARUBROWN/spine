package resolver

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
)

type DTOResolver struct{}

func (r *DTOResolver) Supports(pm ParameterMeta) bool {
	// ExecutionContext 제외
	if pm.Type == reflect.TypeFor[core.ExecutionContext]() {
		return false
	}

	// 반드시 포인터
	if pm.Type.Kind() != reflect.Ptr {
		return false
	}

	elem := pm.Type.Elem()
	if elem.Kind() != reflect.Struct {
		return false
	}

	// form 태그가 하나라도 있으면 FormDTO로 넘긴다
	for i := 0; i < elem.NumField(); i++ {
		if elem.Field(i).Tag.Get("form") != "" {
			return false
		}
	}

	// query 태그가 있으면 QueryDTO
	for i := 0; i < elem.NumField(); i++ {
		if elem.Field(i).Tag.Get("query") != "" {
			return false
		}
	}

	return true
}

func (r *DTOResolver) Resolve(ctx core.RequestContext, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("HTTP 요청 컨텍스트가 아닙니다")
	}

	// 빈 DTO 생성
	valuePtr := reflect.New(parameterMeta.Type)

	if err := httpCtx.Bind(valuePtr.Interface()); err != nil {
		return nil, fmt.Errorf(
			"DTO 바인딩 실패 (%s): %w",
			parameterMeta.Type.Name(),
			err,
		)
	}

	// 포인터가 아니라 값으로 전달
	return valuePtr.Elem().Interface(), nil
}
