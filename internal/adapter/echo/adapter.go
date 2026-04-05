package echo

import (
	"context"
	"net/http"
	"time"

	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
)

const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 30 * time.Second
	defaultWriteTimeout      = 30 * time.Second
	defaultIdleTimeout       = 120 * time.Second
	defaultMaxHeaderBytes    = 1 << 20
	defaultMaxBodyBytes      = 32 << 20
)

type normalizedHTTPOptions struct {
	DisableRecover    bool
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
	MaxBodyBytes      int64
}

type Server struct {
	echo           *echo.Echo
	pipeline       *pipeline.Pipeline
	address        string
	transportHooks []func(any)
	httpServer     *http.Server
	maxBodyBytes   int64
}

func NewServer(pipeline *pipeline.Pipeline, address string, transportHooks []func(any), opts boot.HTTPOptions) *Server {
	normalized := normalizeHTTPOptions(opts)
	e := newEcho(normalized)
	for _, hook := range transportHooks {
		hook(e)
	}

	httpServer := &http.Server{
		Addr:              address,
		Handler:           e,
		ReadHeaderTimeout: normalized.ReadHeaderTimeout,
		ReadTimeout:       normalized.ReadTimeout,
		WriteTimeout:      normalized.WriteTimeout,
		IdleTimeout:       normalized.IdleTimeout,
		MaxHeaderBytes:    normalized.MaxHeaderBytes,
	}

	return &Server{
		echo:           e,
		pipeline:       pipeline,
		address:        address,
		transportHooks: transportHooks,
		httpServer:     httpServer,
		maxBodyBytes:   normalized.MaxBodyBytes,
	}
}

func normalizeHTTPOptions(opts boot.HTTPOptions) normalizedHTTPOptions {
	normalized := normalizedHTTPOptions{
		DisableRecover:    opts.DisableRecover,
		ReadHeaderTimeout: opts.ReadHeaderTimeout,
		ReadTimeout:       opts.ReadTimeout,
		WriteTimeout:      opts.WriteTimeout,
		IdleTimeout:       opts.IdleTimeout,
		MaxHeaderBytes:    opts.MaxHeaderBytes,
		MaxBodyBytes:      opts.MaxBodyBytes,
	}

	if normalized.ReadHeaderTimeout == 0 {
		normalized.ReadHeaderTimeout = defaultReadHeaderTimeout
	}
	if normalized.ReadTimeout == 0 {
		normalized.ReadTimeout = defaultReadTimeout
	}
	if normalized.WriteTimeout == 0 {
		normalized.WriteTimeout = defaultWriteTimeout
	}
	if normalized.IdleTimeout == 0 {
		normalized.IdleTimeout = defaultIdleTimeout
	}
	if normalized.MaxHeaderBytes == 0 {
		normalized.MaxHeaderBytes = defaultMaxHeaderBytes
	}
	if normalized.MaxBodyBytes == 0 {
		normalized.MaxBodyBytes = defaultMaxBodyBytes
	}

	return normalized
}

func newEcho(opts normalizedHTTPOptions) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Logger.SetLevel(log.ERROR)
	if !opts.DisableRecover {
		e.Use(simpleRecover())
	}
	return e
}

// simpleRecover는 외부 의존 없이 panic을 500으로 변환하는 최소한의 미들웨어입니다.
func simpleRecover() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			defer func() {
				if r := recover(); r != nil {
					c.Logger().Errorf("panic recovered: %v", r)
					_ = c.JSON(http.StatusInternalServerError, map[string]any{
						"message": "Internal server error",
					})
				}
			}()
			return next(c)
		}
	}
}

func (s *Server) Mount() {
	s.echo.Any("/*", s.handle)
}

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) handle(c echo.Context) error {
	if s.maxBodyBytes > 0 {
		req := c.Request()
		req.Body = http.MaxBytesReader(c.Response(), req.Body, s.maxBodyBytes)
		c.SetRequest(req)
	}

	ctx := NewContext(c)

	ctx.Set(
		"spine.response_writer",
		NewEchoResponseWriter(c),
	)

	if err := s.pipeline.Execute(ctx); err != nil {
		c.Logger().Errorf("pipeline error: %v", err)
		// 파이프라인 내부에서 이미 응답이 작성되었으므로 Echo 기본 에러 핸들러로 중복 전달하지 않는다.
		return nil
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
