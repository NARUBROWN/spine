package cors

import (
	"strings"

	"github.com/NARUBROWN/spine/core"
)

type Config struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	AllowCredentials bool
}

func New(config Config) *CORSInterceptor {
	return &CORSInterceptor{
		config: config,
	}
}

type CORSInterceptor struct {
	config Config
}

func (i *CORSInterceptor) PreHandle(
	ctx core.ExecutionContext,
	meta core.HandlerMeta,
) error {
	// ResponseWriter 획득
	rwAny, ok := ctx.Get("spine.response_writer")
	if !ok {
		return nil
	}
	rw, ok := rwAny.(core.ResponseWriter)
	if !ok {
		return nil
	}

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

	// Preflight 요청 처리
	if ctx.Method() == "OPTIONS" {
		rw.WriteStatus(204)
		return core.ErrAbortPipeline
	}

	return nil
}

func (i *CORSInterceptor) PostHandle(ctx core.ExecutionContext, meta core.HandlerMeta) {}

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
