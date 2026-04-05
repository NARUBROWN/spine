package handler

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

type RedirectReturnValueHandler struct{}

func (h *RedirectReturnValueHandler) Supports(returnType reflect.Type) bool {
	if returnType.Kind() == reflect.Pointer {
		returnType = returnType.Elem()
	}

	return returnType == reflect.TypeFor[httpx.Redirect]()
}

func (h *RedirectReturnValueHandler) Handle(value any, ctx core.ExecutionContext) error {
	var redirect httpx.Redirect
	switch v := value.(type) {
	case httpx.Redirect:
		redirect = v
	case *httpx.Redirect:
		if v == nil {
			return fmt.Errorf("RedirectReturnValueHandler: nil *httpx.Redirect는 처리할 수 없습니다")
		}
		redirect = *v
	default:
		return fmt.Errorf("RedirectReturnValueHandler: 전달된 값이 httpx.Redirect 타입이 아닙니다")
	}

	rwAny, ok := ctx.Get("spine.response_writer")
	if !ok {
		return fmt.Errorf("ExecutionContext 안에서 ResponseWriter를 찾을 수 없습니다.")
	}

	rw, ok := rwAny.(core.ResponseWriter)
	if !ok {
		return fmt.Errorf("ResponseWriter 타입이 올바르지 않습니다.")
	}

	for k, v := range redirect.Options.Headers {
		rw.SetHeader(k, v)
	}

	for _, c := range redirect.Options.Cookies {
		rw.AddHeader("Set-Cookie", serializeCookie(c))
	}

	rw.SetHeader("Location", redirect.Location)

	status := redirect.Options.Status
	if status == 0 {
		status = http.StatusFound // 302
	}

	return rw.WriteStatus(status)
}

func serializeCookie(c httpx.Cookie) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("%s=%s", c.Name, c.Value))

	if c.Path != "" {
		parts = append(parts, "Path="+c.Path)
	}
	if c.Domain != "" {
		parts = append(parts, "Domain="+c.Domain)
	}
	if c.MaxAge != 0 {
		parts = append(parts, fmt.Sprintf("Max-Age=%d", c.MaxAge))
	}
	if c.Expires != nil {
		parts = append(parts, "Expires="+c.Expires.UTC().Format(time.RFC1123))
	}
	if c.HttpOnly {
		parts = append(parts, "HttpOnly")
	}
	if c.Secure {
		parts = append(parts, "Secure")
	}
	if c.SameSite != "" {
		parts = append(parts, "SameSite="+string(c.SameSite))
	}
	if c.Priority != "" {
		parts = append(parts, "Priority="+c.Priority)
	}

	return strings.Join(parts, "; ")
}
