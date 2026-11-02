package handler

import (
	"encoding/json"
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
			return fmt.Errorf("JSONReturnHandler: cannot handle nil *httpx.Response[*]")
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return fmt.Errorf("JSONReturnHandler: value must be a struct")
	}

	bodyField := val.FieldByName("Body")
	if !bodyField.IsValid() {
		return fmt.Errorf("JSONReturnHandler: Body field not found")
	}

	body := bodyField.Interface()

	optionsField := val.FieldByName("Options")
	if !optionsField.IsValid() {
		return fmt.Errorf("JSONReturnHandler: Options field not found")
	}

	options := optionsField.Interface().(httpx.ResponseOptions)

	rw, writerErr := responseWriter(ctx)
	if writerErr != nil {
		return writerErr
	}

	serializedCookies, err := serializeCookiesValidated(options.Cookies)
	if err != nil {
		return fmt.Errorf("JSONReturnHandler: %w", err)
	}

	for k, v := range options.Headers {
		rw.SetHeader(k, v)
	}

	for _, cookie := range serializedCookies {
		rw.AddHeader("Set-Cookie", cookie)
	}

	status := options.Status
	if status == 0 {
		status = http.StatusOK
	}

	// 동적 반환값은 HTTP 출력 계약으로 넘기기 전에 직렬화합니다.
	if err := ValidateStatus(status); err != nil {
		return err
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("serialize JSON response: %w", err)
	}
	return rw.WriteJSON(status, encoded)
}
