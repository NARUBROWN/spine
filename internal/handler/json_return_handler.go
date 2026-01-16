package handler

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
)

type JSONReturnHandler struct{}

func (h *JSONReturnHandler) Supports(returnType reflect.Type) bool {
	switch returnType.Kind() {
	case reflect.Struct, reflect.Map, reflect.Slice:
		return true
	default:
		return false
	}
}

func (h *JSONReturnHandler) Handle(value any, ctx core.ExecutionContext) error {
	rwAny, ok := ctx.Get("spine.response_writer")
	if !ok {
		return fmt.Errorf("ExecutionContext 안에서 ResponseWriter를 찾을 수 없습니다.")
	}

	rw, ok := rwAny.(core.ResponseWriter)
	if !ok {
		return fmt.Errorf("ResponseWriter 타입이 올바르지 않습니다.")
	}

	return rw.WriteJSON(200, value)
}
