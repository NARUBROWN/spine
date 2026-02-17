package handler

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

type StringReturnHandler struct{}

func (h *StringReturnHandler) Supports(returnType reflect.Type) bool {
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

	return field.Type.Kind() == reflect.String
}

func (h *StringReturnHandler) Handle(value any, ctx core.ExecutionContext) error {
	var resp httpx.Response[string]

	switch v := value.(type) {
	case httpx.Response[string]:
		resp = v
	case *httpx.Response[string]:
		if v == nil {
			return fmt.Errorf("StringReturnHandler: nil *httpx.Response[string]는 처리할 수 없습니다")
		}
		resp = *v
	default:
		return fmt.Errorf("StringReturnHandler: 전달된 값이 httpx.Response[string] 또는 *httpx.Response[string] 타입이 아닙니다: %T", value)
	}

	rwAny, ok := ctx.Get("spine.response_writer")
	if !ok {
		return fmt.Errorf("ExecutionContext 안에서 ResponseWriter를 찾을 수 없습니다.")
	}

	rw, ok := rwAny.(core.ResponseWriter)
	if !ok {
		return fmt.Errorf("ResponseWriter 타입이 올바르지 않습니다.")
	}

	for k, v := range resp.Options.Headers {
		rw.SetHeader(k, v)
	}

	for _, c := range resp.Options.Cookies {
		rw.AddHeader("Set-Cookie", serializeCookie(c))
	}

	status := resp.Options.Status
	if status == 0 {
		status = http.StatusOK
	}

	return rw.WriteString(status, resp.Body)
}
