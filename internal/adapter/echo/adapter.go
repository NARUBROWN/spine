package echo

import (
	"context"
	"net/http"

	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
)

type Server struct {
	echo           *echo.Echo
	pipeline       *pipeline.Pipeline
	address        string
	transportHooks []func(any)
}

func NewServer(pipeline *pipeline.Pipeline, address string, transportHooks []func(any), recoverEnabled bool) *Server {
	e := newEcho(recoverEnabled)
	for _, hook := range transportHooks {
		hook(e)
	}

	return &Server{
		echo:           e,
		pipeline:       pipeline,
		address:        address,
		transportHooks: transportHooks,
	}
}

func newEcho(recoverEnabled bool) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Logger.SetLevel(log.ERROR)
	if recoverEnabled {
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
	return s.echo.Start(s.address)
}

func (s *Server) handle(c echo.Context) error {
	ctx := NewContext(c)

	ctx.Set(
		"spine.response_writer",
		NewEchoResponseWriter(c),
	)

	if err := s.pipeline.Execute(ctx); err != nil {
		c.Logger().Errorf("pipeline error: %v", err)
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}
