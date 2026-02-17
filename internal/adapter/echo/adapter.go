package echo

import (
	"context"

	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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
		e.Use(middleware.Recover())
	}
	return e
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
