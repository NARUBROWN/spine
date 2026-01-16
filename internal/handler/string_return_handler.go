package handler

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
)

type StringReturnHandler struct{}

func (h *StringReturnHandler) Supports(returnType reflect.Type) bool {
	return returnType.Kind() == reflect.String
}

func (h *StringReturnHandler) Handle(value any, ctx core.ExecutionContext) error {
	rwAny, ok := ctx.Get("spine.response_writer")
	if !ok {
		return fmt.Errorf("ExecutionContext 안에서 ResponseWriter를 찾을 수 없습니다.")
	}

	rw, ok := rwAny.(core.ResponseWriter)
	if !ok {
		return fmt.Errorf("ResponseWriter 타입이 올바르지 않습니다.")
	}

	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("StringReturnHandler는 string 타입만 처리할 수 있습니다: %T", value)
	}

	return rw.WriteString(200, str)
}
