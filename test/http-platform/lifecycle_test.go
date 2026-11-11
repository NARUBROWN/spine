package httpplatform_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	gorillaPlatform "github.com/NARUBROWN/spine-gorilla-platform"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httperr"
	handlerspine "github.com/NARUBROWN/spine/pkg/spine"
	ws "github.com/NARUBROWN/spine/pkg/ws"
	"github.com/gorilla/websocket"
)

func TestPreparedCloseDrainsNativeAfterProcessing(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		t.Run(engine, func(t *testing.T) {
			entered, release := make(chan context.Context, 1), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{after: func(r *http.Request) { entered <- r.Context(); <-release }})))
			app.Constructor(func() *inputController { return &inputController{} })
			app.Route("POST", "/body", (*inputController).Body)
			p := prepare(t, app, boot.HTTPOptions{})
			server := externalServer(t, p)
			t.Cleanup(unblock)
			client := &http.Client{Timeout: 5 * time.Second}
			t.Cleanup(client.CloseIdleConnections)
			responseDone := make(chan error, 1)
			go func() {
				r, err := client.Post(server.URL+"/body", "application/json", strings.NewReader(`{"name":"active"}`))
				if err == nil {
					_, err = io.Copy(io.Discard, r.Body)
					_ = r.Body.Close()
					if r.StatusCode != 200 {
						err = errors.New("active request status")
					}
				}
				responseDone <- err
			}()
			var active context.Context
			select {
			case active = <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("native 후처리에 진입하지 않았습니다")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if err := p.Close(ctx); !errors.Is(err, context.DeadlineExceeded) || active.Err() != nil {
				t.Fatalf("대기 기한과 진행 요청 보존: %v %v", err, active.Err())
			}
			response, err := client.Get(server.URL + "/body")
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode != 503 {
				t.Fatalf("새 진입 상태: %d", response.StatusCode)
			}
			unblock()
			select {
			case err := <-responseDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("진행 요청 완료 시간 초과")
			}
			ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel2()
			if err := p.Close(ctx2); err != nil || active.Err() != context.Canceled {
				t.Fatalf("재대기와 요청 종료 취소: %v %v", err, active.Err())
			}
		})
	}
}

type websocketController struct{}
type websocketBody struct {
	Message string `json:"message"`
}

func (*websocketController) Message(ctx context.Context, dto websocketBody) {
	body, _ := json.Marshal(dto)
	_ = ws.Send(ctx, ws.TextMessage, body)
}

func TestWebSocketAndRuntimeOpenAPIOverTCP(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		t.Run(engine, func(t *testing.T) {
			app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{})), gorillaPlatform.Option())
			app.Constructor(func() *websocketController { return &websocketController{} }, func() *inputController { return &inputController{} })
			app.Route("POST", "/body", (*inputController).Body)
			if err := app.WebSocket().Register("/ws", (*websocketController).Message); err != nil {
				t.Fatal(err)
			}
			p := prepare(t, app, boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true, Title: "Platform E2E", Version: "local"}})
			url, client := managedServer(t, p)
			for _, method := range []string{"GET", "HEAD"} {
				r, _ := http.NewRequest(method, url+"/openapi.json", nil)
				response, err := client.Do(r)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if err != nil || response.StatusCode != 200 {
					t.Fatalf("OpenAPI: %d %v", response.StatusCode, err)
				}
				if method == "HEAD" {
					if len(body) != 0 {
						t.Fatal("HEAD 응답 본문이 있습니다")
					}
					continue
				}
				var document struct {
					OpenAPI string                     `json:"openapi"`
					Paths   map[string]json.RawMessage `json:"paths"`
				}
				if err := json.Unmarshal(body, &document); err != nil || document.OpenAPI != "3.1.2" || document.Paths["/body"] == nil {
					t.Fatalf("runtime 명세: %s %v", body, err)
				}
			}
			conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(url, "http")+"/ws", nil)
			if err != nil {
				t.Fatalf("WebSocket 업그레이드: response=%v err=%v", response, err)
			}
			defer conn.Close()
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"message":"hello"}`)); err != nil {
				t.Fatal(err)
			}
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, payload, err := conn.ReadMessage()
			if err != nil || canonical(payload) != `{"message":"hello"}` {
				t.Fatalf("WebSocket 메시지: %s %v", payload, err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := p.Close(ctx); err != nil {
				t.Fatal(err)
			}
			if _, _, err := conn.ReadMessage(); err == nil {
				t.Fatal("앱 종료 뒤 WebSocket 연결이 남았습니다")
			}
		})
	}
}

func (*websocketController) Restricted(ctx handlerspine.Ctx) error {
	return ws.Send(ctx.ExecutionContext().Context(), ws.TextMessage, []byte("restricted"))
}

type websocketRouteAuthorization struct{}

func (websocketRouteAuthorization) PreHandshake(_ handlerspine.Ctx, meta core.HandlerMeta) error {
	if meta.MethodName == "Restricted" {
		return &httperr.HTTPError{Status: http.StatusForbidden, Message: "Forbidden"}
	}
	return nil
}
func (websocketRouteAuthorization) PreHandle(handlerspine.Ctx, core.HandlerMeta) error { return nil }
func (websocketRouteAuthorization) PostHandle(handlerspine.Ctx, core.HandlerMeta)      {}
func (websocketRouteAuthorization) BeforeResponse(handlerspine.Ctx, core.HandlerMeta, error) error {
	return nil
}
func (websocketRouteAuthorization) AfterCompletion(handlerspine.Ctx, core.HandlerMeta, error) {}

func TestWebSocketExecutesTheHandlerAuthorizedAtUpgrade(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		t.Run(engine, func(t *testing.T) {
			app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{})), gorillaPlatform.Option())
			app.Constructor(func() *websocketController { return &websocketController{} })
			app.InterceptorFor(boot.InterceptorWebSocket, websocketRouteAuthorization{})
			if err := app.WebSocket().Register("/ws", (*websocketController).Message); err != nil {
				t.Fatal(err)
			}
			if err := app.WebSocket().Register("/ws/", (*websocketController).Restricted); err != nil {
				t.Fatal(err)
			}
			if err := app.WebSocket().Register("/ws", (*websocketController).Restricted); err == nil {
				t.Fatal("같은 WebSocket 경로를 다시 등록했습니다")
			}
			prepared := prepare(t, app, boot.HTTPOptions{})
			base, _ := managedServer(t, prepared)
			wsBase := "ws" + strings.TrimPrefix(base, "http")
			denied, response, err := websocket.DefaultDialer.Dial(wsBase+"/ws/", nil)
			if denied != nil {
				_ = denied.Close()
			}
			if response != nil {
				defer response.Body.Close()
			}
			if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
				t.Fatalf("제한된 핸들러의 업그레이드 결과: %v %v", response, err)
			}
			conn, _, err := websocket.DefaultDialer.Dial(wsBase+"/ws", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteJSON(websocketBody{Message: "authorized"}); err != nil {
				t.Fatal(err)
			}
			_, payload, err := conn.ReadMessage()
			if err != nil || canonical(payload) != `{"message":"authorized"}` {
				t.Fatalf("승인된 핸들러의 메시지: %s %v", payload, err)
			}
		})
	}
}
