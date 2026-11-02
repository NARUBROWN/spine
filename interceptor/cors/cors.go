package cors

import (
	"errors"
	"fmt"
	"strings"

	"github.com/NARUBROWN/spine/core"
)

const (
	ConfigErrorCodeWildcardOriginWithCredentials = "CORS_CREDENTIALS_WITH_WILDCARD_ORIGIN"
	ConfigErrorPathAllowOrigins                  = "CORS.AllowOrigins"
)

var ErrWildcardOriginWithCredentials = errors.New("CORS credentials cannot be used with wildcard origin")

// ConfigError는 시작 단계의 검증에서 오류 문자열을 파싱하지 않고도 보고할 수 있는 형태로
// 잘못된 CORS 설정을 설명합니다.
type ConfigError struct {
	Code    string
	Path    string
	Message string
	Hint    string
	Err     error
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("%s: %s (%s)", e.Path, e.Message, e.Code)
}

func (e *ConfigError) Unwrap() error { return e.Err }

type Config struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	AllowCredentials bool
}

// New는 기존 생성자 시그니처를 유지하면서 CORS 인터셉터를 생성합니다. 잘못된 설정은
// 응답 헤더를 쓰기 전에 PreHandle에서 보고됩니다. 새 코드는 요청 처리를 시작하기 전에
// 시작 단계에서 실패할 수 있도록 NewValidated를 우선 사용해야 합니다.
func New(config Config) *CORSInterceptor {
	interceptor, err := NewValidated(config)
	if err != nil {
		// 기존 생성자의 소스 호환성을 유지합니다. 잘못된 정책은 응답 헤더를 쓰기 전에
		// PreHandle에서 검증 오류를 반환할 수 있도록 보관합니다.
		return &CORSInterceptor{config: config, configErr: err}
	}
	return interceptor
}

// NewValidated는 CORS 인터셉터를 생성하거나 안전하지 않은 정책에 대한 구조화된 오류를 반환합니다.
// 새 코드는 New보다 이 생성자를 우선 사용해야 합니다.
func NewValidated(config Config) (*CORSInterceptor, error) {
	if err := ValidateConfig(config); err != nil {
		return nil, err
	}
	return &CORSInterceptor{config: config}, nil
}

// ValidateConfig는 인터셉터를 생성하지 않고 CORS 정책을 검증합니다.
// 애플리케이션 시작 단계의 사전 검사에서 사용할 수 있습니다.
func ValidateConfig(config Config) error {
	if config.AllowCredentials && containsWildcardOrigin(config.AllowOrigins) {
		return &ConfigError{
			Code:    ConfigErrorCodeWildcardOriginWithCredentials,
			Path:    ConfigErrorPathAllowOrigins,
			Message: "AllowCredentials cannot be enabled when AllowOrigins contains wildcard origin \"*\"",
			Hint:    "replace \"*\" with explicit trusted origins or disable AllowCredentials",
			Err:     ErrWildcardOriginWithCredentials,
		}
	}
	return nil
}

type CORSInterceptor struct {
	config    Config
	configErr error
}

func (i *CORSInterceptor) PreHandle(
	ctx core.ExecutionContext,
	meta core.HandlerMeta,
) error {
	if i.configErr != nil {
		return i.configErr
	}

	// ResponseWriter 가져오기
	response, ok := ctx.(core.HTTPResponseContext)
	if !ok || response.ResponseWriter() == nil {
		return nil
	}
	rw := response.ResponseWriter()

	origin := ctx.Header("Origin")
	if origin != "" && i.isAllowedOrigin(origin) {
		rw.SetHeader("Access-Control-Allow-Origin", origin)
		rw.SetHeader("Vary", "Origin")
	}

	rw.SetHeader(
		"Access-Control-Allow-Methods",
		strings.Join(i.config.AllowMethods, ", "),
	)

	rw.SetHeader(
		"Access-Control-Allow-Headers",
		strings.Join(i.config.AllowHeaders, ", "),
	)

	if i.config.AllowCredentials {
		rw.SetHeader("Access-Control-Allow-Credentials", "true")
	}

	// Origin과 대상 메서드가 있는 실제 CORS 사전 요청만 여기서 종료한다.
	// 일반 OPTIONS 요청은 등록된 애플리케이션 핸들러로 전달한다.
	if ctx.Method() == "OPTIONS" && origin != "" && ctx.Header("Access-Control-Request-Method") != "" {
		rw.WriteStatus(204)
		return core.ErrAbortPipeline
	}

	return nil
}

func (i *CORSInterceptor) PostHandle(ctx core.ExecutionContext, meta core.HandlerMeta) {}

func (i *CORSInterceptor) BeforeResponse(
	ctx core.ExecutionContext,
	meta core.HandlerMeta,
	executionErr error,
) error {
	return nil
}

func (i *CORSInterceptor) AfterCompletion(ctx core.ExecutionContext, meta core.HandlerMeta, err error) {
}

func (i *CORSInterceptor) isAllowedOrigin(origin string) bool {
	for _, o := range i.config.AllowOrigins {
		if o == "*" || o == origin {
			return true
		}
	}
	return false
}

func containsWildcardOrigin(origins []string) bool {
	for _, origin := range origins {
		if origin == "*" {
			return true
		}
	}
	return false
}
