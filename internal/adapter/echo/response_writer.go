package echo

import (
	"github.com/labstack/echo/v4"
)

type EchoResponseWriter struct {
	ctx echo.Context
}

func NewEchoResponseWriter(ctx echo.Context) *EchoResponseWriter {
	return &EchoResponseWriter{ctx: ctx}
}

func (w *EchoResponseWriter) WriteJSON(status int, value any) error {
	return w.ctx.JSON(status, value)
}

func (w *EchoResponseWriter) WriteString(status int, value string) error {
	return w.ctx.String(status, value)
}

func (w *EchoResponseWriter) SetHeader(key, value string) {
	w.ctx.Response().Header().Set(key, value)
}

func (w *EchoResponseWriter) WriteStatus(status int) error {
	w.ctx.Response().WriteHeader(status)
	return nil
}
