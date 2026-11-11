package cors

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/NARUBROWN/spine/core"
	contextapi "github.com/NARUBROWN/spine/pkg/spine"
)

const (
	ConfigErrorCodeWildcardOriginWithCredentials = "CORS_CREDENTIALS_WITH_WILDCARD_ORIGIN"
	ConfigErrorPathAllowOrigins                  = "CORS.AllowOrigins"
	ConfigErrorCodeInvalidConfig                 = "CORS_INVALID_CONFIG"
)

var ErrWildcardOriginWithCredentials = errors.New("CORS credentials cannot be used with wildcard origin")

type ConfigError struct {
	Code    string
	Path    string
	Message string
	Hint    string
	Err     error
}

func (e *ConfigError) Error() string { return fmt.Sprintf("%s: %s (%s)", e.Path, e.Message, e.Code) }
func (e *ConfigError) Unwrap() error { return e.Err }

type Config struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	AllowCredentials bool
	ExposeHeaders    []string
	// MaxAge는 preflight 캐시 시간의 초 단위 값입니다. nil은 생략, 0은 명시적 0입니다.
	MaxAge *int
	// AllowOrigin은 정적 목록이 출처를 허용하지 않을 때 호출합니다.
	AllowOrigin func(context.Context, string) (bool, error)
}

// New는 기존 생성자를 유지하며 잘못된 설정을 첫 요청에서 보고합니다.
func New(config Config) *CORSInterceptor {
	interceptor, err := NewValidated(config)
	if err != nil {
		return &CORSInterceptor{config: config, configErr: err}
	}
	return interceptor
}

// NewValidated는 시작 단계에서 정책을 검증합니다.
func NewValidated(config Config) (*CORSInterceptor, error) {
	if err := ValidateConfig(config); err != nil {
		return nil, err
	}
	config.AllowOrigins = append([]string(nil), config.AllowOrigins...)
	config.AllowMethods = append([]string(nil), config.AllowMethods...)
	config.AllowHeaders = append([]string(nil), config.AllowHeaders...)
	config.ExposeHeaders = append([]string(nil), config.ExposeHeaders...)
	if config.MaxAge != nil {
		value := *config.MaxAge
		config.MaxAge = &value
	}
	return &CORSInterceptor{config: config}, nil
}

func invalidConfig(path, message string) error {
	return &ConfigError{Code: ConfigErrorCodeInvalidConfig, Path: path, Message: message, Hint: "provide valid nonempty HTTP tokens and a nonnegative max age"}
}

// ValidateConfig는 네트워크 작업 없이 CORS 정책을 검증합니다.
func ValidateConfig(config Config) error {
	if config.AllowCredentials && contains(config.AllowOrigins, "*", false) {
		return &ConfigError{Code: ConfigErrorCodeWildcardOriginWithCredentials, Path: ConfigErrorPathAllowOrigins, Message: "AllowCredentials cannot be enabled when AllowOrigins contains wildcard origin \"*\"", Hint: "replace \"*\" with explicit trusted origins or disable AllowCredentials", Err: ErrWildcardOriginWithCredentials}
	}
	if config.MaxAge != nil && *config.MaxAge < 0 {
		return invalidConfig("CORS.MaxAge", "max age must be nonnegative")
	}
	for _, item := range []struct {
		name   string
		values []string
	}{{"AllowMethods", config.AllowMethods}, {"AllowHeaders", config.AllowHeaders}, {"ExposeHeaders", config.ExposeHeaders}} {
		for _, value := range item.values {
			if !httpToken(value) {
				return invalidConfig("CORS."+item.name, "invalid HTTP token")
			}
		}
	}
	for _, origin := range config.AllowOrigins {
		if origin == "" || strings.ContainsAny(origin, "\r\n\t ") {
			return invalidConfig("CORS.AllowOrigins", "invalid origin")
		}
	}
	return nil
}

type CORSInterceptor struct {
	config    Config
	configErr error
}

func (i *CORSInterceptor) PreHandle(ctx contextapi.Ctx, meta core.HandlerMeta) error {
	if i.configErr != nil {
		return i.configErr
	}
	request, ok := ctx.HTTPRequestContext()
	if !ok {
		return nil
	}
	rw, ok := ctx.HTTPResponseContext()
	if !ok {
		return nil
	}
	origin := request.Header("Origin")
	requestedMethod := request.Header("Access-Control-Request-Method")
	preflight := request.Method() == http.MethodOptions && origin != "" && requestedMethod != ""
	// AddHeader는 기존 Vary를 지우지 않습니다. 여러 Vary 헤더는 같은 목록입니다.
	rw.AddHeader("Vary", "Origin")
	if request.Method() == http.MethodOptions {
		rw.AddHeader("Vary", "Access-Control-Request-Method")
		rw.AddHeader("Vary", "Access-Control-Request-Headers")
	}
	allowed, err := i.allowedOrigin(ctx.ExecutionContext().Context(), origin)
	if err != nil {
		return fmt.Errorf("CORS origin decision: %w", err)
	}
	if preflight {
		requestedHeaders, valid := parseRequestedHeaders(request.Header("Access-Control-Request-Headers"))
		if !allowed || !httpToken(requestedMethod) || !i.allowedMethod(requestedMethod) || !valid || !i.allowedHeaders(requestedHeaders) {
			if err := rw.WriteStatus(http.StatusForbidden); err != nil {
				return err
			}
			return core.ErrAbortPipeline
		}
	}
	if allowed {
		rw.SetHeader("Access-Control-Allow-Origin", origin)
		if i.config.AllowCredentials {
			rw.SetHeader("Access-Control-Allow-Credentials", "true")
		}
		if preflight {
			rw.SetHeader("Access-Control-Allow-Methods", strings.Join(i.config.AllowMethods, ", "))
			rw.SetHeader("Access-Control-Allow-Headers", strings.Join(i.config.AllowHeaders, ", "))
			if i.config.MaxAge != nil {
				rw.SetHeader("Access-Control-Max-Age", strconv.Itoa(*i.config.MaxAge))
			}
		} else if len(i.config.ExposeHeaders) > 0 {
			rw.SetHeader("Access-Control-Expose-Headers", strings.Join(i.config.ExposeHeaders, ", "))
		}
	}
	if preflight {
		if err := rw.WriteStatus(http.StatusNoContent); err != nil {
			return err
		}
		return core.ErrAbortPipeline
	}
	return nil
}

func (i *CORSInterceptor) allowedOrigin(ctx context.Context, origin string) (bool, error) {
	if origin == "" {
		return false, nil
	}
	if contains(i.config.AllowOrigins, origin, false) || origin != "null" && contains(i.config.AllowOrigins, "*", false) {
		return true, nil
	}
	if i.config.AllowOrigin != nil {
		return i.config.AllowOrigin(ctx, origin)
	}
	return false, nil
}

func (i *CORSInterceptor) allowedMethod(method string) bool {
	return contains(i.config.AllowMethods, method, false) || !i.config.AllowCredentials && contains(i.config.AllowMethods, "*", false)
}

func (i *CORSInterceptor) allowedHeaders(headers []string) bool {
	wildcard := !i.config.AllowCredentials && contains(i.config.AllowHeaders, "*", false)
	for _, h := range headers {
		if contains(i.config.AllowHeaders, h, true) {
			continue
		}
		if !wildcard || strings.EqualFold(h, "Authorization") {
			return false
		}
	}
	return true
}

func contains(values []string, value string, fold bool) bool {
	for _, v := range values {
		if v == value || fold && strings.EqualFold(v, value) {
			return true
		}
	}
	return false
}

func parseRequestedHeaders(value string) ([]string, bool) {
	if value == "" {
		return nil, true
	}
	var headers []string
	for _, part := range strings.Split(value, ",") {
		name := strings.TrimSpace(part)
		if !httpToken(name) {
			return nil, false
		}
		headers = append(headers, name)
	}
	return headers, true
}

func httpToken(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c) {
			continue
		}
		return false
	}
	return true
}

func (i *CORSInterceptor) PostHandle(contextapi.Ctx, core.HandlerMeta) {}
func (i *CORSInterceptor) BeforeResponse(contextapi.Ctx, core.HandlerMeta, error) error {
	return nil
}
func (i *CORSInterceptor) AfterCompletion(contextapi.Ctx, core.HandlerMeta, error) {}
