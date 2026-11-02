package cors

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/NARUBROWN/spine/core"
)

type testExecutionContext struct {
	writer  core.ResponseWriter
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
func (w *testResponseWriter) WriteJSON(status int, value json.RawMessage) error {
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
	ctx.headers["Access-Control-Request-Method"] = "POST"

	writer := newTestResponseWriter()
	ctx.SetResponseWriter(writer)

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

func TestCORSInterceptor_OrdinaryOptionsContinuesPipeline(t *testing.T) {
	interceptor := New(Config{
		AllowOrigins: []string{"https://app.example"},
	})
	ctx := newTestExecutionContext("OPTIONS")
	writer := newTestResponseWriter()
	ctx.SetResponseWriter(writer)

	if err := interceptor.PreHandle(ctx, core.HandlerMeta{}); err != nil {
		t.Fatalf("일반 OPTIONS 요청은 계속 진행해야 합니다: %v", err)
	}
	if writer.status != 0 {
		t.Fatalf("일반 OPTIONS 요청에 조기 응답하면 안 됩니다: %d", writer.status)
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
	ctx.SetResponseWriter(writer)

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

func TestCORSInterceptor_WildcardNeverEnablesCredentials(t *testing.T) {
	config := Config{
		AllowOrigins:     []string{"*"},
		AllowCredentials: true,
	}

	interceptor, err := NewValidated(config)
	if interceptor != nil {
		t.Fatal("invalid wildcard policy must not construct an interceptor")
	}
	if !errors.Is(err, ErrWildcardOriginWithCredentials) {
		t.Fatalf("expected wildcard credentials validation error: %v", err)
	}
	var configErr *ConfigError
	if !errors.As(err, &configErr) {
		t.Fatalf("expected structured ConfigError: %T", err)
	}
	if configErr.Code != ConfigErrorCodeWildcardOriginWithCredentials || configErr.Path != ConfigErrorPathAllowOrigins || configErr.Hint == "" {
		t.Fatalf("unexpected structured error: %+v", configErr)
	}

	// 기존 생성자는 소스 호환성을 유지하면서도 응답 헤더를 내보내기 전에
	// 동일한 검증 오류를 보고합니다.
	interceptor = New(config)
	ctx := newTestExecutionContext("GET")
	ctx.headers["Origin"] = "https://attacker.example"
	writer := newTestResponseWriter()
	ctx.SetResponseWriter(writer)
	if err := interceptor.PreHandle(ctx, core.HandlerMeta{}); !errors.Is(err, ErrWildcardOriginWithCredentials) {
		t.Fatalf("legacy constructor must surface validation error: %v", err)
	}
	if len(writer.headers) != 0 {
		t.Fatalf("invalid policy must not emit any CORS headers: %v", writer.headers)
	}
}

func TestCORSInterceptor_NoResponseWriterIsNoop(t *testing.T) {
	interceptor := New(Config{})
	ctx := newTestExecutionContext("GET")

	if err := interceptor.PreHandle(ctx, core.HandlerMeta{}); err != nil {
		t.Fatalf("ResponseWriter가 없으면 no-op 이어야 합니다: %v", err)
	}
}

func (c *testExecutionContext) SetRoute(params map[string]string, pathKeys []string) {}

func (c *testExecutionContext) ResponseWriter() core.ResponseWriter          { return c.writer }
func (c *testExecutionContext) SetResponseWriter(writer core.ResponseWriter) { c.writer = writer }
