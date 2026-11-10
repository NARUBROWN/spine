package test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platform "github.com/NARUBROWN/spine-echo-platform"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
	contextapi "github.com/NARUBROWN/spine/pkg/spine"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"
)

type transportBaselineCounter struct {
	calls atomic.Int32
}

func (c *transportBaselineCounter) PreHandle(contextapi.Ctx, core.HandlerMeta) error {
	c.calls.Add(1)
	return nil
}

func (*transportBaselineCounter) PostHandle(contextapi.Ctx, core.HandlerMeta) {}

func (*transportBaselineCounter) BeforeResponse(contextapi.Ctx, core.HandlerMeta, error) error {
	return nil
}

func (*transportBaselineCounter) AfterCompletion(contextapi.Ctx, core.HandlerMeta, error) {}

func TestAppIntegration_NativeRouteBaseline(t *testing.T) {
	counter := &transportBaselineCounter{}
	var hookOrder []string
	app := setupHTTPBaselineApp(platform.WithEcho(func(engine *echo.Echo) {
		hookOrder = append(hookOrder, "first")
		engine.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(ctx *echo.Context) error {
				ctx.Response().Header().Set("X-Echo-Middleware", "applied")
				return next(ctx)
			}
		})
		engine.GET("/api/body", func(ctx *echo.Context) error { return ctx.String(200, "first") })
	}),
		platform.WithEcho(func(engine *echo.Echo) {
			hookOrder = append(hookOrder, "second")
			engine.GET("/api/body", func(ctx *echo.Context) error { return ctx.String(200, "second") })
			engine.POST("/native/body", func(ctx *echo.Context) error {
				content, err := io.ReadAll(ctx.Request().Body)
				if err != nil {
					return err
				}
				return ctx.String(200, strconv.Itoa(len(content)))
			})
		}))
	app.InterceptorFor(boot.InterceptorHTTP, counter)

	handler := newHTTPBaselineHandler(t, app, boot.HTTPOptions{GlobalPrefix: "/api", MaxBodyBytes: 32})
	if got := strings.Join(hookOrder, ","); got != "first,second" {
		t.Fatalf("사용자 훅은 등록 순서대로 실행해야 합니다: %s", got)
	}
	cases := []struct {
		name, method, target, body string
		status                     int
		spineCalls                 int32
		wantBody                   string
	}{
		{name: "spine_prefix", method: "GET", target: "/api/items/43", status: 200, spineCalls: 1},
		{name: "unprefixed_spine_route", method: "GET", target: "/items/43", status: 404, spineCalls: 1},
		{name: "last_native_route_overrides_spine", method: "GET", target: "/api/body", status: 200, wantBody: "second"},
		{name: "spine_body_limit", method: "POST", target: "/api/body", body: `{"name":"` + strings.Repeat("x", 64) + `"}`, status: 413, spineCalls: 1},
		{name: "native_body_bypasses_spine_limit", method: "POST", target: "/native/body", body: strings.Repeat("x", 64), status: 200, wantBody: "64"},
		{name: "native_route_has_no_automatic_prefix", method: "POST", target: "/api/native/body", status: 404, spineCalls: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			before := counter.calls.Load()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != tc.status {
				t.Fatalf("상태=%d 기대값=%d 본문=%s", recorder.Code, tc.status, recorder.Body.String())
			}
			if got := counter.calls.Load() - before; got != tc.spineCalls {
				t.Errorf("Spine HTTP 인터셉터 호출=%d 기대값=%d", got, tc.spineCalls)
			}
			if got := recorder.Header().Get("X-Echo-Middleware"); got != "applied" {
				t.Errorf("모든 경로에 Echo 미들웨어가 적용되어야 합니다: %q", got)
			}
			if tc.wantBody != "" && recorder.Body.String() != tc.wantBody {
				t.Errorf("native 응답=%q 기대값=%q", recorder.Body.String(), tc.wantBody)
			}
		})
	}
}

func TestAppIntegration_WebSocketMountBaseline(t *testing.T) {
	app := setupWebSocketApp(platform.WithEcho(func(engine *echo.Echo) {
		// 내장 WS 마운트 뒤에 실행되는 사용자 훅이 같은 GET 경로를 대체하는 현재 동작이다.
		engine.GET("/ws/echo", func(ctx *echo.Context) error { return ctx.String(200, "native-ws") })
	}))
	if err := app.WebSocket().Register("/ws/prefix", (*wsIntegrationController).Echo); err != nil {
		t.Fatal(err)
	}
	counter := &transportBaselineCounter{}
	app.InterceptorFor(boot.InterceptorHTTP, counter)
	handler := newHTTPBaselineHandler(t, app, boot.HTTPOptions{GlobalPrefix: "/api"})
	server := httptest.NewServer(handler)
	defer server.Close()
	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	connection, response, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/prefix", nil)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		t.Fatalf("WS 경로에는 HTTP GlobalPrefix가 자동 적용되지 않아야 합니다: %v", err)
	}
	defer connection.Close()
	if err := connection.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := connection.WriteMessage(websocket.TextMessage, []byte(`{"message":"baseline"}`)); err != nil {
		t.Fatal(err)
	}
	_, content, err := connection.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var reply wsEchoResponse
	if err := json.Unmarshal(content, &reply); err != nil || reply.Message != "baseline" {
		t.Fatalf("WS 메시지 처리 결과=%s 오류=%v", content, err)
	}
	if got := counter.calls.Load(); got != 0 {
		t.Fatalf("WS handshake와 메시지는 HTTP 인터셉터를 거치지 않아야 합니다: %d", got)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ws/echo", nil))
	if recorder.Code != 200 || recorder.Body.String() != "native-ws" {
		t.Fatalf("사용자 native 경로가 내장 WS 경로를 대체해야 합니다: 상태=%d 본문=%q", recorder.Code, recorder.Body.String())
	}
	if got := counter.calls.Load(); got != 0 {
		t.Fatalf("native 경로는 HTTP 인터셉터를 거치지 않아야 합니다: %d", got)
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/ws/prefix", nil))
	if recorder.Code != 404 || counter.calls.Load() != 1 {
		t.Fatalf("접두사를 붙인 WS 경로는 HTTP 파이프라인의 404여야 합니다: 상태=%d HTTP 호출=%d", recorder.Code, counter.calls.Load())
	}
}
