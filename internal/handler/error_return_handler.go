package handler

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
)

type ErrorReturnHandler struct{}

func (h *ErrorReturnHandler) Supports(returnType reflect.Type) bool {
	return returnType == reflect.TypeOf((*error)(nil)).Elem()
}

func (h *ErrorReturnHandler) Handle(value any, ctx core.ExecutionContext) error {
	rwAny, ok := ctx.Get("spine.response_writer")
	if !ok {
		return fmt.Errorf("ExecutionContext 안에서 ResponseWriter를 찾을 수 없습니다.")
	}

	rw, ok := rwAny.(core.ResponseWriter)
	if !ok {
		return fmt.Errorf("ResponseWriter 타입이 올바르지 않습니다.")
	}

	err, ok := value.(error)
	if !ok {
		return fmt.Errorf("ErrorReturnHandler는 error 타입만 처리할 수 있습니다: %T", value)
	}

	return rw.WriteJSON(500, map[string]any{
		"message": err.Error(),
	})
}
