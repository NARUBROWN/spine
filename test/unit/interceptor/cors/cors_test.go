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
func (w *testResponseWriter) AddHeader(key, value string) {
	if w.headers[key] != "" {
		w.headers[key] += ", "
	}
	w.headers[key] += value
}
func (w *testResponseWriter) IsCommitted() bool { return false }
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
	if writer.headers["Vary"] != "Origin, Access-Control-Request-Method, Access-Control-Request-Headers" {
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
	if writer.headers["Access-Control-Allow-Methods"] != "" {
		t.Fatalf("허용되지 않은 출처에 정책을 출력했습니다: %v", writer.headers)
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

func TestCORSPreflightPolicy(t *testing.T) {
	maxAge := 0
	tests := []struct {
		name, origin, method, headers string
		config                        Config
		status                        int
	}{
		{"allowed", "https://app.example", "POST", "content-type, X-Request-ID", Config{AllowOrigins: []string{"https://app.example"}, AllowMethods: []string{"POST"}, AllowHeaders: []string{"Content-Type", "X-Request-ID"}, AllowCredentials: true, MaxAge: &maxAge}, 204},
		{"unknown origin", "https://other.example", "POST", "", Config{AllowOrigins: []string{"https://app.example"}, AllowMethods: []string{"POST"}}, 403},
		{"unknown method", "https://app.example", "DELETE", "", Config{AllowOrigins: []string{"https://app.example"}, AllowMethods: []string{"POST"}}, 403},
		{"unknown header", "https://app.example", "POST", "X-Other", Config{AllowOrigins: []string{"https://app.example"}, AllowMethods: []string{"POST"}}, 403},
		{"bad header list", "https://app.example", "POST", "X-One,", Config{AllowOrigins: []string{"https://app.example"}, AllowMethods: []string{"POST"}, AllowHeaders: []string{"X-One"}}, 403},
		{"wildcard", "https://app.example", "POST", "X-Other", Config{AllowOrigins: []string{"*"}, AllowMethods: []string{"*"}, AllowHeaders: []string{"*"}}, 204},
		{"wildcard excludes authorization", "https://app.example", "POST", "Authorization", Config{AllowOrigins: []string{"*"}, AllowMethods: []string{"*"}, AllowHeaders: []string{"*"}}, 403},
		{"explicit authorization", "https://app.example", "POST", "Authorization", Config{AllowOrigins: []string{"*"}, AllowMethods: []string{"*"}, AllowHeaders: []string{"*", "Authorization"}}, 204},
		{"credentialed wildcard method", "https://app.example", "POST", "", Config{AllowOrigins: []string{"https://app.example"}, AllowMethods: []string{"*"}, AllowCredentials: true}, 403},
		{"credentialed wildcard header", "https://app.example", "POST", "X-Test", Config{AllowOrigins: []string{"https://app.example"}, AllowMethods: []string{"POST"}, AllowHeaders: []string{"*"}, AllowCredentials: true}, 403},
		{"null not included by wildcard", "null", "POST", "", Config{AllowOrigins: []string{"*"}, AllowMethods: []string{"POST"}}, 403},
		{"explicit null", "null", "POST", "", Config{AllowOrigins: []string{"null"}, AllowMethods: []string{"POST"}}, 204},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewValidated(tt.config)
			if err != nil {
				t.Fatal(err)
			}
			ctx := newTestExecutionContext("OPTIONS")
			ctx.headers["Origin"], ctx.headers["Access-Control-Request-Method"], ctx.headers["Access-Control-Request-Headers"] = tt.origin, tt.method, tt.headers
			writer := newTestResponseWriter()
			writer.SetHeader("Vary", "Accept-Encoding")
			ctx.SetResponseWriter(writer)
			if err := c.PreHandle(ctx, core.HandlerMeta{}); !errors.Is(err, core.ErrAbortPipeline) {
				t.Fatal(err)
			}
			if writer.status != tt.status {
				t.Fatalf("status=%d", writer.status)
			}
			if writer.headers["Vary"] != "Accept-Encoding, Origin, Access-Control-Request-Method, Access-Control-Request-Headers" {
				t.Fatal(writer.headers)
			}
			if tt.status == 403 && writer.headers["Access-Control-Allow-Origin"] != "" {
				t.Fatal(writer.headers)
			}
			if tt.name == "allowed" && writer.headers["Access-Control-Max-Age"] != "0" {
				t.Fatal(writer.headers)
			}
		})
	}
}

func TestCORSOriginCallbackAndVary(t *testing.T) {
	wantErr := errors.New("origin lookup failed")
	calls := 0
	policy := New(Config{AllowOrigins: []string{"https://known.example"}, ExposeHeaders: []string{"X-Request-ID"}, AllowOrigin: func(_ context.Context, origin string) (bool, error) {
		calls++
		if origin == "https://error.example" {
			return false, wantErr
		}
		return origin == "https://dynamic.example", nil
	}})
	for _, tc := range []struct {
		origin  string
		allowed bool
		count   int
	}{{"", false, 0}, {"https://known.example", true, 0}, {"https://dynamic.example", true, 1}, {"https://other.example", false, 2}} {
		ctx := newTestExecutionContext("GET")
		ctx.headers["Origin"] = tc.origin
		w := newTestResponseWriter()
		w.SetHeader("Vary", "Accept-Encoding")
		ctx.SetResponseWriter(w)
		if err := policy.PreHandle(ctx, core.HandlerMeta{}); err != nil {
			t.Fatal(err)
		}
		if calls != tc.count || w.headers["Vary"] != "Accept-Encoding, Origin" {
			t.Fatal(calls, w.headers)
		}
		if (w.headers["Access-Control-Allow-Origin"] != "") != tc.allowed {
			t.Fatal(w.headers)
		}
		if (w.headers["Access-Control-Expose-Headers"] != "") != tc.allowed {
			t.Fatal(w.headers)
		}
	}
	ctx := newTestExecutionContext("GET")
	ctx.headers["Origin"] = "https://error.example"
	w := newTestResponseWriter()
	ctx.SetResponseWriter(w)
	if err := policy.PreHandle(ctx, core.HandlerMeta{}); !errors.Is(err, wantErr) || w.headers["Vary"] != "Origin" || len(w.headers) != 1 {
		t.Fatal(err, w.headers)
	}
}

func TestCORSConfigValidation(t *testing.T) {
	negative := -1
	for _, config := range []Config{{MaxAge: &negative}, {AllowHeaders: []string{"X-Bad\r\nX-Foo"}}, {AllowMethods: []string{""}}, {ExposeHeaders: []string{"Bad Name"}}, {AllowOrigins: []string{"https://bad.example\nX"}}} {
		_, err := NewValidated(config)
		var detail *ConfigError
		if !errors.As(err, &detail) || detail.Code != ConfigErrorCodeInvalidConfig {
			t.Fatalf("%+v: %v", config, err)
		}
	}
}
