package handler

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

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
			return fmt.Errorf("RedirectReturnValueHandler: cannot handle nil *httpx.Redirect")
		}
		redirect = *v
	default:
		return fmt.Errorf("RedirectReturnValueHandler: value is not an httpx.Redirect")
	}

	rw, writerErr := responseWriter(ctx)
	if writerErr != nil {
		return writerErr
	}

	serializedCookies, err := serializeCookiesValidated(redirect.Options.Cookies)
	if err != nil {
		return fmt.Errorf("RedirectReturnValueHandler: %w", err)
	}

	for k, v := range redirect.Options.Headers {
		rw.SetHeader(k, v)
	}

	for _, cookie := range serializedCookies {
		rw.AddHeader("Set-Cookie", cookie)
	}

	rw.SetHeader("Location", redirect.Location)

	status := redirect.Options.Status
	if status == 0 {
		status = http.StatusFound // 302
	}

	return rw.WriteStatus(status)
}

func serializeCookiesValidated(cookies []httpx.Cookie) ([]string, error) {
	serialized := make([]string, 0, len(cookies))
	for index, cookie := range cookies {
		value, err := serializeCookieValidated(cookie)
		if err != nil {
			return nil, fmt.Errorf("invalid cookie at index %d: %w", index, err)
		}
		serialized = append(serialized, value)
	}
	return serialized, nil
}

func serializeCookieValidated(c httpx.Cookie) (string, error) {
	var parts []string

	if c.Name == "" {
		return "", fmt.Errorf("cookie Name must not be empty")
	}
	if err := validateCookieToken("Name", c.Name); err != nil {
		return "", err
	}
	if err := validateCookieValue("Value", c.Value); err != nil {
		return "", err
	}
	parts = append(parts, fmt.Sprintf("%s=%s", c.Name, c.Value))

	if c.Path != "" {
		if err := validateCookieValue("Path", c.Path); err != nil {
			return "", err
		}
		parts = append(parts, "Path="+c.Path)
	}
	if c.Domain != "" {
		if err := validateCookieValue("Domain", c.Domain); err != nil {
			return "", err
		}
		parts = append(parts, "Domain="+c.Domain)
	}
	if c.MaxAge > 0 {
		parts = append(parts, fmt.Sprintf("Max-Age=%d", c.MaxAge))
	} else if c.MaxAge < 0 {
		parts = append(parts, "Max-Age=0")
	}
	if c.Expires != nil {
		parts = append(parts, "Expires="+c.Expires.UTC().Format(http.TimeFormat))
	}
	if c.HttpOnly {
		parts = append(parts, "HttpOnly")
	}
	if c.Secure {
		parts = append(parts, "Secure")
	}
	if c.SameSite != "" {
		switch c.SameSite {
		case httpx.SameSiteLax, httpx.SameSiteStrict, httpx.SameSiteNone:
		default:
			return "", fmt.Errorf("cookie SameSite must be one of Lax, Strict, or None: %q", c.SameSite)
		}
		if err := validateCookieToken("SameSite", string(c.SameSite)); err != nil {
			return "", err
		}
		parts = append(parts, "SameSite="+string(c.SameSite))
	}
	if c.Priority != "" {
		switch c.Priority {
		case "Low", "Medium", "High":
		default:
			return "", fmt.Errorf("cookie Priority must be one of Low, Medium, or High: %q", c.Priority)
		}
		if err := validateCookieToken("Priority", c.Priority); err != nil {
			return "", err
		}
		parts = append(parts, "Priority="+c.Priority)
	}

	return strings.Join(parts, "; "), nil
}

func validateCookieToken(field, value string) error {
	for index, r := range value {
		if r <= 0x20 || r >= 0x7f || r == ';' || r == ',' || r == '=' {
			return fmt.Errorf("cookie %s contains invalid character %q at byte %d", field, r, index)
		}
	}
	return nil
}

func validateCookieValue(field, value string) error {
	for index, r := range value {
		if r <= 0x20 || r >= 0x7f || r == ';' || r == ',' {
			return fmt.Errorf("cookie %s contains invalid character %q at byte %d", field, r, index)
		}
	}
	return nil
}
