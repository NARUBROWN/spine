package handler

import (
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

func (h *JSONReturnHandler) Handle(value any, ctx core.Context) error {
	return ctx.JSON(200, value)
}
