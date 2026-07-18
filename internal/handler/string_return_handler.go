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
			return fmt.Errorf("StringReturnHandler: cannot handle nil *httpx.Response[string]")
		}
		resp = *v
	default:
		return fmt.Errorf("StringReturnHandler: value is not httpx.Response[string] or *httpx.Response[string]: %T", value)
	}

	rwAny, ok := ctx.Get("spine.response_writer")
	if !ok {
		return fmt.Errorf("ResponseWriter not found in ExecutionContext")
	}

	rw, ok := rwAny.(core.ResponseWriter)
	if !ok {
		return fmt.Errorf("invalid ResponseWriter type")
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
