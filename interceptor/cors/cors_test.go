package cors

import (
	"context"
	"errors"
	"testing"

	"github.com/NARUBROWN/spine/core"
)

type testExecutionContext struct {
	method  string
	headers map[string]string
	store   map[string]any
}

func newTestExecutionContext(method string) *testExecutionContext {
	return &testExecutionContext{
		method:  method,
		headers: map[string]string{},
		store:   map[string]any{},
	}
}

func (c *testExecutionContext) Context() context.Context     { return context.Background() }
func (c *testExecutionContext) EventBus() core.EventBus      { return nil }
func (c *testExecutionContext) Method() string               { return c.method }
func (c *testExecutionContext) Path() string                 { return "/" }
func (c *testExecutionContext) Params() map[string]string    { return map[string]string{} }
func (c *testExecutionContext) Header(name string) string    { return c.headers[name] }
func (c *testExecutionContext) PathKeys() []string           { return nil }
func (c *testExecutionContext) Queries() map[string][]string { return map[string][]string{} }
func (c *testExecutionContext) Set(key string, value any)    { c.store[key] = value }
func (c *testExecutionContext) Get(key string) (any, bool)   { v, ok := c.store[key]; return v, ok }

type testResponseWriter struct {
	headers map[string]string
	status  int
}

func newTestResponseWriter() *testResponseWriter {
	return &testResponseWriter{headers: map[string]string{}}
}

func (w *testResponseWriter) SetHeader(key, value string) { w.headers[key] = value }
func (w *testResponseWriter) AddHeader(key, value string) { w.headers[key] = value }
func (w *testResponseWriter) IsCommitted() bool           { return false }
func (w *testResponseWriter) WriteStatus(status int) error {
	w.status = status
	return nil
}
func (w *testResponseWriter) WriteJSON(status int, value any) error {
	w.status = status
	return nil
}
func (w *testResponseWriter) WriteString(status int, value string) error {
	w.status = status
	return nil
}
func (w *testResponseWriter) WriteBytes(status int, value []byte) error {
	w.status = status
	return nil
}

func TestCORSInterceptor_PreflightAllowedOrigin(t *testing.T) {
	interceptor := New(Config{
		AllowOrigins:     []string{"https://app.example"},
		AllowMethods:     []string{"GET", "POST"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	})

	ctx := newTestExecutionContext("OPTIONS")
	ctx.headers["Origin"] = "https://app.example"

	writer := newTestResponseWriter()
	ctx.Set("spine.response_writer", writer)

	err := interceptor.PreHandle(ctx, core.HandlerMeta{})
	if !errors.Is(err, core.ErrAbortPipeline) {
		t.Fatalf("preflight는 파이프라인을 중단해야 합니다: %v", err)
	}

	if writer.status != 204 {
		t.Fatalf("preflight 상태 코드는 204여야 합니다: %d", writer.status)
	}
	if writer.headers["Access-Control-Allow-Origin"] != "https://app.example" {
		t.Fatalf("Allow-Origin 헤더가 잘못되었습니다: %v", writer.headers)
	}
	if writer.headers["Vary"] != "Origin" {
		t.Fatalf("Vary 헤더가 누락되었습니다: %v", writer.headers)
	}
	if writer.headers["Access-Control-Allow-Credentials"] != "true" {
		t.Fatalf("Allow-Credentials 헤더가 누락되었습니다: %v", writer.headers)
	}
	if writer.headers["Access-Control-Allow-Methods"] != "GET, POST" {
		t.Fatalf("Allow-Methods 헤더가 잘못되었습니다: %v", writer.headers)
	}
	if writer.headers["Access-Control-Allow-Headers"] != "Content-Type, Authorization" {
		t.Fatalf("Allow-Headers 헤더가 잘못되었습니다: %v", writer.headers)
	}
}

func TestCORSInterceptor_DisallowedOriginOmitsAllowOrigin(t *testing.T) {
	interceptor := New(Config{
		AllowOrigins: []string{"https://app.example"},
		AllowMethods: []string{"GET"},
		AllowHeaders: []string{"Content-Type"},
	})

	ctx := newTestExecutionContext("GET")
	ctx.headers["Origin"] = "https://evil.example"

	writer := newTestResponseWriter()
	ctx.Set("spine.response_writer", writer)

	if err := interceptor.PreHandle(ctx, core.HandlerMeta{}); err != nil {
		t.Fatalf("예상하지 못한 에러입니다: %v", err)
	}

	if _, ok := writer.headers["Access-Control-Allow-Origin"]; ok {
		t.Fatalf("허용되지 않은 origin에는 Allow-Origin 헤더가 없어야 합니다: %v", writer.headers)
	}
	if writer.headers["Access-Control-Allow-Methods"] != "GET" {
		t.Fatalf("Allow-Methods는 항상 설정되어야 합니다: %v", writer.headers)
	}
}

func TestCORSInterceptor_NoResponseWriterIsNoop(t *testing.T) {
	interceptor := New(Config{})
	ctx := newTestExecutionContext("GET")

	if err := interceptor.PreHandle(ctx, core.HandlerMeta{}); err != nil {
		t.Fatalf("ResponseWriter가 없으면 no-op 이어야 합니다: %v", err)
	}
}
