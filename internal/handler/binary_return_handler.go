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
	binary, ok := value.(httpx.Binary)
	if !ok {
		return fmt.Errorf("BinaryReturnValueHandler: 전달된 값이 httpx.Binary 타입이 아닙니다")
	}

	rwAny, ok := ctx.Get("spine.response_writer")
	if !ok {
		return fmt.Errorf("ExecutionContext 안에서 ResponseWriter를 찾을 수 없습니다.")
	}

	rw, ok := rwAny.(core.ResponseWriter)
	if !ok {
		return fmt.Errorf("ResponseWriter 타입이 올바르지 않습니다.")
	}

	// 사용자 정의 헤더 설정
	for k, v := range binary.Options.Headers {
		rw.SetHeader(k, v)
	}

	// 쿠키 설정
	for _, c := range binary.Options.Cookies {
		rw.AddHeader("Set-Cookie", serializeCookie(c))
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
