package handler

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

type JSONReturnHandler struct{}

func (h *JSONReturnHandler) Supports(returnType reflect.Type) bool {

	if returnType.Kind() == reflect.Pointer {
		returnType = returnType.Elem()
	}

	if returnType.Kind() != reflect.Struct {
		return false
	}

	if returnType.PkgPath() != "github.com/NARUBROWN/spine/pkg/httpx" {
		return false
	}

	if !strings.HasPrefix(returnType.Name(), "Response[") {
		return false
	}

	field, ok := returnType.FieldByName("Body")
	if !ok {
		return false
	}

	return field.Type.Kind() != reflect.String
}

func (h *JSONReturnHandler) Handle(value any, ctx core.ExecutionContext) error {
	val := reflect.ValueOf(value)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return fmt.Errorf("JSONReturnHandler: nil *httpx.Response[*]는 처리할 수 없습니다")
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return fmt.Errorf("JSONReturnHandler: value는 struct여야 합니다")
	}

	bodyField := val.FieldByName("Body")
	if !bodyField.IsValid() {
		return fmt.Errorf("JSONReturnHandler: Body 필드를 찾을 수 없습니다")
	}

	body := bodyField.Interface()

	optionsField := val.FieldByName("Options")
	if !optionsField.IsValid() {
		return fmt.Errorf("JSONReturnHandler: Options 필드를 찾을 수 없습니다")
	}

	options := optionsField.Interface().(httpx.ResponseOptions)

	rwAny, ok := ctx.Get("spine.response_writer")
	if !ok {
		return fmt.Errorf("ExecutionContext 안에서 ResponseWriter를 찾을 수 없습니다.")
	}

	rw, ok := rwAny.(core.ResponseWriter)
	if !ok {
		return fmt.Errorf("ResponseWriter 타입이 올바르지 않습니다.")
	}

	for k, v := range options.Headers {
		rw.SetHeader(k, v)
	}

	for _, c := range options.Cookies {
		rw.AddHeader("Set-Cookie", serializeCookie(c))
	}

	status := options.Status
	if status == 0 {
		status = http.StatusOK
	}

	return rw.WriteJSON(status, body)
}
