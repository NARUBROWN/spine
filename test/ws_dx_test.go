package test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	echo "github.com/NARUBROWN/spine-echo-platform"
	gorilla "github.com/NARUBROWN/spine-gorilla-platform"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
	sc "github.com/NARUBROWN/spine/pkg/spine"
	"github.com/NARUBROWN/spine/pkg/ws"
	"github.com/gorilla/websocket"
)

type dxMessage struct {
	Text string `json:"text"`
}
type dxService struct{ sender ws.Sender }

func (*dxService) Constructor(sender ws.Sender) *dxService { return &dxService{sender} }
func (s *dxService) Send(ctx context.Context, id, text string) error {
	return s.sender.Send(ctx, id, ws.TextMessage, []byte(text))
}

type dxController struct {
	service   *dxService
	connected chan string
	frames    chan dxFrame
}

type dxConfig struct {
	service *dxService
	ids     chan string
	frames  chan dxFrame
	calls   atomic.Int32
}

func (*dxController) Constructor(s *dxService, config *dxConfig) *dxController {
	config.service = s
	return &dxController{service: s, connected: config.ids, frames: config.frames}
}

func (c *dxController) RegisterRoutes(r *api.Registry) {
	r.Group("/chat", api.WebSocketInterceptors((*dxInterceptor)(nil))).WS("/ws", func(ctx ws.MessageContext, id ws.ConnectionID, message input.Message[dxMessage]) error {
		if value, ok := ctx.Get("ws-audit"); !ok || value != true {
			return errors.New("WebSocket handler did not see the interceptor value")
		}
		if c.frames != nil {
			c.frames <- dxFrame{ctx: ctx, payload: message.Value.Text}
		}
		c.connected <- id.Value
		return c.service.Send(ctx, id.Value, message.Value.Text)
	})
	r.POST("/push/:id", func(ctx sc.Ctx, id input.Path[string], body input.Body[dxMessage]) (httpx.Response[string], error) {
		err := c.service.Send(ctx.ExecutionContext().Context(), *id, body.Text)
		return httpx.Response[string]{Body: "sent"}, err
	})
}

func TestWSDX_CallbackMessageAndDISenderOverNetwork(t *testing.T) {
	var service *dxService
	ids := make(chan string, 4)
	app := spine.New(echo.Option(), gorilla.Option())
	config := &dxConfig{ids: ids}
	app.Constructor(constructor.Custom(func() *dxConfig { return config }), constructor.Service(&dxService{}), constructor.Interceptor(&dxInterceptor{}), constructor.Controller(&dxController{}))
	prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{GlobalPrefix: "/api"}})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close(context.Background())
	service = config.service
	handler, err := spine.NetHTTPHandler(prepared)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/chat/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := conn.WriteJSON(dxMessage{Text: "echo"}); err != nil {
		t.Fatal(err)
	}
	id := <-ids
	if config.calls.Load() == 0 {
		t.Fatal("DI message interceptor did not run")
	}
	_, data, err := conn.ReadMessage()
	if err != nil || string(data) != "echo" {
		t.Fatalf("echo=%s err=%v", data, err)
	}
	response, err := http.Post(server.URL+"/api/push/"+id, "application/json", strings.NewReader(`{"text":"http"}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	_, data, err = conn.ReadMessage()
	if err != nil || string(data) != "http" {
		t.Fatalf("http=%s err=%v", data, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := service.Send(ctx, id, "canceled"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	const count = 16
	var sends sync.WaitGroup
	for i := range count {
		sends.Add(1)
		go func() {
			defer sends.Done()
			if err := service.Send(t.Context(), id, fmt.Sprint(i)); err != nil {
				t.Error(err)
			}
		}()
	}
	for range count {
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatal(err)
		}
	}
	sends.Wait()
	if err := service.Send(t.Context(), "missing", "x"); !errors.Is(err, ws.ErrConnectionNotFound) {
		t.Fatal(err)
	}

	conn.Close()
	removed := false
	until := time.Now().Add(2 * time.Second)
	for time.Now().Before(until) {
		if errors.Is(service.Send(t.Context(), id, "after disconnect"), ws.ErrConnectionNotFound) {
			removed = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !removed {
		t.Fatal("disconnected connection stayed registered")
	}
	next, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/chat/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	next.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := next.WriteJSON(dxMessage{Text: "reconnected"}); err != nil {
		t.Fatal(err)
	}
	nextID := <-ids
	if nextID == id {
		t.Fatal("reconnected socket reused the old connection identity")
	}
	if _, data, err := next.ReadMessage(); err != nil || string(data) != "reconnected" {
		t.Fatalf("reconnect=%s err=%v", data, err)
	}
	if err := prepared.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.Send(t.Context(), id, "x"); !errors.Is(err, ws.ErrSenderClosed) {
		t.Fatal(err)
	}
}

type dxInvalidController struct{ variant string }

func (c *dxInvalidController) RegisterRoutes(r *api.Registry) {
	switch c.variant {
	case "http-interceptor-on-websocket":
		r.WS("/ws", func(context.Context) error { return nil }, api.Interceptors(&responseBaselineInterceptor{}))
	case "websocket-interceptor-on-http":
		r.GET("/status", func() httpx.Response[string] { return httpx.Response[string]{Body: "ok"} }, api.WebSocketInterceptors(&dxInterceptor{}))
	case "spine-context":
		r.WS("/ws", func(sc.Ctx) error { return nil })
	case "naked":
		r.WS("/ws", func(message dxMessage) error { return nil })
	case "type":
		r.WS("/ws", func(message input.Message[int]) error { return nil })
	case "collision":
		r.WS("/ws", func() error { return nil })
		r.GET("/ws", func() httpx.Response[string] { return httpx.Response[string]{Body: "x"} })
	default:
		r.WS("/ws", func() error { return nil })
	}
}
func TestWSDX_PreparationValidatesCollectedCallbacksAndPlatforms(t *testing.T) {
	for _, variant := range []string{"naked", "type", "spine-context", "collision", "missing-platform"} {
		t.Run(variant, func(t *testing.T) {
			options := []spine.Option{echo.Option()}
			if variant != "missing-platform" {
				options = append(options, gorilla.Option())
			}
			app := spine.New(options...)
			app.Constructor(func() *dxInvalidController { return &dxInvalidController{variant} })
			p, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{}})
			if err == nil {
				p.Close(context.Background())
				t.Fatal("invalid callback was prepared")
			}
		})
	}
}

type dxInterceptor struct{ config *dxConfig }

func (*dxInterceptor) Constructor(config *dxConfig) *dxInterceptor {
	return &dxInterceptor{config: config}
}
func (i *dxInterceptor) PreMessage(ctx ws.MessageContext, _ core.HandlerMeta) error {
	if _, exists := ctx.Get("ws-audit"); exists {
		return errors.New("WebSocket message context retained a value from a previous frame")
	}
	i.config.calls.Add(1)
	ctx.Set("ws-audit", true)
	return nil
}
func (*dxInterceptor) PostMessage(ws.MessageContext, core.HandlerMeta) {}
func (*dxInterceptor) BeforeMessageCompletion(ws.MessageContext, core.HandlerMeta, error) error {
	return nil
}
func (*dxInterceptor) AfterMessageCompletion(ws.MessageContext, core.HandlerMeta, error) {}

type dxFrame struct {
	ctx     context.Context
	payload string
}

func TestWSDX_StandardContextsCancelPerMessageWithoutClosingConnection(t *testing.T) {
	frames := make(chan dxFrame, 2)
	config := &dxConfig{ids: make(chan string, 2), frames: frames}
	app := spine.New(echo.Option(), gorilla.Option())
	app.Constructor(constructor.Custom(func() *dxConfig { return config }), constructor.Service(&dxService{}), constructor.Interceptor(&dxInterceptor{}), constructor.Controller(&dxController{}))
	prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{}})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close(context.Background())
	handler, err := spine.NetHTTPHandler(prepared)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/chat/ws?token=snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var first context.Context
	for _, text := range []string{"first", "second"} {
		if err := conn.WriteJSON(dxMessage{Text: text}); err != nil {
			t.Fatal(err)
		}
		var frame dxFrame
		select {
		case frame = <-frames:
		case <-time.After(3 * time.Second):
			t.Fatal("handler did not run")
		}
		if frame.payload != text {
			t.Fatal("wrong received input")
		}
		_, payload, err := conn.ReadMessage()
		if err != nil || string(payload) != text {
			t.Fatalf("echo=%s err=%v", payload, err)
		}
		messageCtx := frame.ctx
		select {
		case <-messageCtx.Done():
		case <-time.After(3 * time.Second):
			t.Fatal("message context remains active")
		}
		if first == messageCtx {
			t.Fatal("message context reused")
		}
		first = messageCtx
		if config.calls.Load() == 0 {
			t.Fatal("DI message interceptor did not run")
		}
	}
}

type legacyWSCtxController struct{}

func (*legacyWSCtxController) Handle(sc.Ctx) error { return nil }
func TestWSDXRejectsSpineContextInLegacyRegistration(t *testing.T) {
	app := spine.New(echo.Option(), gorilla.Option())
	app.Constructor(func() *legacyWSCtxController { return &legacyWSCtxController{} })
	if err := app.WebSocket().Register("/ws", (*legacyWSCtxController).Handle); err != nil {
		t.Fatal(err)
	}
	prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{}})
	if err == nil {
		prepared.Close(context.Background())
		t.Fatal("legacy spine.Ctx handler accepted")
	}
	if !strings.Contains(err.Error(), "WEBSOCKET_PARAMETER_UNSUPPORTED") {
		t.Fatal(err)
	}
}

// An old auth hook must never disappear silently during signature migration.
type legacyHandshakeAuth struct{ dxInterceptor }

func (*legacyHandshakeAuth) PreHandshake(sc.Ctx, core.HandlerMeta) error { return errors.New("deny") }

type legacyHandshakeController struct{ route bool }

func (c *legacyHandshakeController) RegisterRoutes(r *api.Registry) {
	if c.route {
		r.WS("/ws", func(context.Context) error { return nil }, api.WebSocketInterceptors(&legacyHandshakeAuth{}))
	} else {
		r.WS("/ws", func(context.Context) error { return nil })
	}
}
func TestWSDXRejectsIncompatibleHandshakeAuthBeforeServing(t *testing.T) {
	for _, route := range []bool{false, true} {
		t.Run(fmt.Sprint(route), func(t *testing.T) {
			app := spine.New(echo.Option(), gorilla.Option())
			app.Constructor(func() *legacyHandshakeController { return &legacyHandshakeController{route: route} })
			if !route {
				app.WebSocketInterceptor(&legacyHandshakeAuth{})
			}
			prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{}})
			if err == nil {
				prepared.Close(context.Background())
				t.Fatal("incompatible auth hook was ignored")
			}
			if !strings.Contains(err.Error(), "WEBSOCKET_HANDSHAKE_SIGNATURE_UNSUPPORTED") {
				t.Fatal(err)
			}
		})
	}
}

func TestWSDX_RejectsMismatchedInterceptorRegistration(t *testing.T) {
	for _, tt := range []struct {
		name              string
		controllerVariant string
		register          func(spine.App)
		diagnostic        string
	}{
		{name: "HTTP route option on WebSocket", controllerVariant: "http-interceptor-on-websocket", diagnostic: "use api.WebSocketInterceptors"},
		{name: "WebSocket route option on HTTP", controllerVariant: "websocket-interceptor-on-http", diagnostic: "use api.Interceptors"},
		{
			name: "legacy WebSocket scope",
			register: func(app spine.App) {
				app.InterceptorFor(boot.InterceptorWebSocket, &responseBaselineInterceptor{})
			},
			diagnostic: "WEBSOCKET_INTERCEPTOR_REGISTRATION_REQUIRED",
		},
		{
			name: "legacy shared scope",
			register: func(app spine.App) {
				app.Interceptor(&responseBaselineInterceptor{})
			},
			diagnostic: "WEBSOCKET_INTERCEPTOR_REGISTRATION_REQUIRED",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := spine.New(echo.Option(), gorilla.Option())
			app.Constructor(func() *dxInvalidController {
				return &dxInvalidController{variant: tt.controllerVariant}
			})
			if tt.register != nil {
				tt.register(app)
			}
			prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{}})
			if err == nil {
				_ = prepared.Close(context.Background())
				t.Fatal("Prepare accepted a mismatched interceptor registration")
			}
			if !strings.Contains(err.Error(), tt.diagnostic) {
				t.Fatalf("Prepare error = %v, want %s", err, tt.diagnostic)
			}
		})
	}
}
