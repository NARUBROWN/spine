package handler

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

type BinaryReturnHandler struct{}

func (h *BinaryReturnHandler) Supports(returnType reflect.Type) bool {
	if returnType.Kind() == reflect.Pointer {
		returnType = returnType.Elem()
	}

	return returnType == reflect.TypeFor[httpx.Binary]()
}

func (h *BinaryReturnHandler) Handle(value any, ctx core.ExecutionContext) error {
	var binary httpx.Binary
	switch v := value.(type) {
	case httpx.Binary:
		binary = v
	case *httpx.Binary:
		if v == nil {
			return fmt.Errorf("BinaryReturnValueHandler: cannot handle nil *httpx.Binary")
		}
		binary = *v
	default:
		return fmt.Errorf("BinaryReturnValueHandler: value is not an httpx.Binary")
	}

	rw, writerErr := responseWriter(ctx)
	if writerErr != nil {
		return writerErr
	}

	serializedCookies, err := serializeCookiesValidated(binary.Options.Cookies)
	if err != nil {
		return fmt.Errorf("BinaryReturnValueHandler: %w", err)
	}

	// 사용자 정의 헤더 설정
	for k, v := range binary.Options.Headers {
		rw.SetHeader(k, v)
	}

	// 쿠키 설정
	for _, cookie := range serializedCookies {
		rw.AddHeader("Set-Cookie", cookie)
	}

	// Content-Type 설정
	if binary.ContentType != "" {
		rw.SetHeader("Content-Type", binary.ContentType)
	}

	status := binary.Options.Status
	if status == 0 {
		status = http.StatusOK
	}

	return rw.WriteBytes(status, binary.Data)
}
