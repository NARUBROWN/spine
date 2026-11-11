package test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gorillaplatform "github.com/NARUBROWN/spine-gorilla-platform"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/NARUBROWN/spine/internal/resolver"
	spinerouter "github.com/NARUBROWN/spine/internal/router"
	wsruntime "github.com/NARUBROWN/spine/internal/ws"
	"github.com/NARUBROWN/spine/pkg/boot"
	contextapi "github.com/NARUBROWN/spine/pkg/spine"
	pkgws "github.com/NARUBROWN/spine/pkg/ws"
	"github.com/gorilla/websocket"
)

type requestAwareInterceptor struct {
	handshakeSeen chan struct{}
	messageSeen   chan struct{}
	handshakeAuth string
	messageAuth   string
	messageQuery  string
	messageCookie string
	messageRemote string
}

func (i *requestAwareInterceptor) PreHandshake(ctx contextapi.Ctx, _ core.HandlerMeta) error {
	request, _ := ctx.WebSocket()
	i.handshakeAuth = request.Header("Authorization")
	close(i.handshakeSeen)
	return nil
}

func (i *requestAwareInterceptor) PreHandle(ctx contextapi.Ctx, _ core.HandlerMeta) error {
	request, _ := ctx.WebSocket()
	i.messageAuth = request.Header("Authorization")
	i.messageQuery = request.Queries()["token"][0]
	if requestCtx, ok := ctx.WebSocket(); ok {
		i.messageCookie, _ = requestCtx.Cookie("session")
		i.messageRemote = requestCtx.RemoteAddr()
	}
	close(i.messageSeen)
	return nil
}

func (*requestAwareInterceptor) PostHandle(contextapi.Ctx, core.HandlerMeta) {}
func (*requestAwareInterceptor) BeforeResponse(contextapi.Ctx, core.HandlerMeta, error) error {
	return nil
}
func (*requestAwareInterceptor) AfterCompletion(contextapi.Ctx, core.HandlerMeta, error) {}

type cancellationController struct {
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

type stopWaitController struct {
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
	finished chan struct{}
}

func (c *stopWaitController) Wait(ctx context.Context) {
	close(c.started)
	<-ctx.Done()
	close(c.canceled)
	<-c.release
	close(c.finished)
}

func (c *cancellationController) Wait(ctx context.Context) {
	close(c.started)
	select {
	case <-ctx.Done():
		close(c.canceled)
	case <-c.release:
	}
}

type concurrentSendController struct {
	errs chan error
}

func (c *concurrentSendController) Burst(ctx context.Context) {
	const messages = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range messages {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			if err := pkgws.Send(ctx, pkgws.TextMessage, []byte(fmt.Sprintf("message-%d", index))); err != nil {
				select {
				case c.errs <- err:
				default:
				}
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(c.errs)
}

func TestRuntime_StopCancelsActiveHandlerAndRejectsNewConnections(t *testing.T) {
	controller := &cancellationController{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
	runtime, registration := newTestRuntime(t, controller, (*cancellationController).Wait, boot.WebSocketOptions{
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		PingInterval: time.Second,
	})
	defer close(controller.release)

	server := newRuntimeTestServer(runtime, registration)
	defer server.Close()

	conn := dialRuntimeTestServer(t, server)
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("start")); err != nil {
		t.Fatalf("메시지 전송 실패: %v", err)
	}
	select {
	case <-controller.started:
	case <-time.After(time.Second):
		t.Fatal("WebSocket 핸들러가 시작되지 않았습니다")
	}

	runtime.Stop()

	select {
	case <-controller.canceled:
	case <-time.After(time.Second):
		t.Fatal("wsruntime.Runtime.Stop이 활성 핸들러의 context를 취소하지 않았습니다")
	}

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	_, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("중지된 런타임은 신규 연결을 거부해야 합니다")
	}
	if response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("중지된 런타임은 503을 반환해야 합니다: %v", response)
	}
}

func TestRuntime_StopWaitsForActiveHandlerToReturn(t *testing.T) {
	controller := &stopWaitController{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
		finished: make(chan struct{}),
	}
	runtime, registration := newTestRuntime(t, controller, (*stopWaitController).Wait, boot.WebSocketOptions{
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		PingInterval: time.Second,
	})

	server := newRuntimeTestServer(runtime, registration)
	defer server.Close()
	conn := dialRuntimeTestServer(t, server)
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("start")); err != nil {
		t.Fatalf("메시지 전송 실패: %v", err)
	}
	select {
	case <-controller.started:
	case <-time.After(time.Second):
		t.Fatal("WebSocket 핸들러가 시작되지 않았습니다")
	}

	stopped := make(chan struct{})
	go func() {
		runtime.Stop()
		close(stopped)
	}()
	select {
	case <-controller.canceled:
	case <-time.After(time.Second):
		t.Fatal("wsruntime.Runtime.Stop이 활성 핸들러의 context를 취소하지 않았습니다")
	}
	select {
	case <-stopped:
		t.Fatal("wsruntime.Runtime.Stop은 활성 핸들러가 반환되기 전에 종료되면 안 됩니다")
	case <-time.After(100 * time.Millisecond):
	}

	close(controller.release)
	select {
	case <-controller.finished:
	case <-time.After(time.Second):
		t.Fatal("취소된 WebSocket 핸들러가 반환되지 않았습니다")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("활성 핸들러 반환 후 wsruntime.Runtime.Stop이 종료되지 않았습니다")
	}
}

func TestRuntime_ConcurrentSendsRemainSafeWhilePingRuns(t *testing.T) {
	controller := &concurrentSendController{errs: make(chan error, 1)}
	runtime, registration := newTestRuntime(t, controller, (*concurrentSendController).Burst, boot.WebSocketOptions{
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		PingInterval: time.Millisecond,
	})
	defer runtime.Stop()

	server := newRuntimeTestServer(runtime, registration)
	defer server.Close()
	conn := dialRuntimeTestServer(t, server)
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte("burst")); err != nil {
		t.Fatalf("메시지 전송 실패: %v", err)
	}

	seen := make(map[string]struct{}, 32)
	for len(seen) < 32 {
		if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatalf("read deadline 설정 실패: %v", err)
		}
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("동시 전송 응답 수신 실패 (%d/32): %v", len(seen), err)
		}
		if messageType != websocket.TextMessage {
			t.Fatalf("예상하지 못한 메시지 타입: %d", messageType)
		}
		seen[string(payload)] = struct{}{}
	}
	for {
		select {
		case err, ok := <-controller.errs:
			if !ok {
				return
			}
			if err != nil {
				t.Fatalf("동시 전송 실패: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("동시 WebSocket 전송 결과 대기가 시간 내 종료되지 않았습니다")
		}
	}
}

func TestRuntime_RejectsCapacityBeforeWebSocketUpgrade(t *testing.T) {
	controller := &cancellationController{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
	runtime, registration := newTestRuntime(t, controller, (*cancellationController).Wait, boot.WebSocketOptions{
		MaxConnections:     1,
		CapacityRetryAfter: 3 * time.Second,
	})
	defer runtime.Stop()
	defer close(controller.release)

	server := newRuntimeTestServer(runtime, registration)
	defer server.Close()
	first := dialRuntimeTestServer(t, server)
	defer first.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	_, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("connection above capacity must be rejected")
	}
	if response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("capacity rejection must return HTTP 503 before upgrade: %v", response)
	}
	defer response.Body.Close()
	if got := response.Header.Get("Retry-After"); got != "3" {
		t.Fatalf("Retry-After = %q, want 3", got)
	}
}

func TestRuntime_CommonInterceptorReadsHandshakeCredentialsFromMessageContext(t *testing.T) {
	controller := &cancellationController{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
	interceptor := &requestAwareInterceptor{
		handshakeSeen: make(chan struct{}),
		messageSeen:   make(chan struct{}),
	}
	runtime, registration := newTestRuntime(t, controller, (*cancellationController).Wait, boot.WebSocketOptions{}, interceptor)
	defer runtime.Stop()

	server := newRuntimeTestServer(runtime, registration)
	defer server.Close()
	defer close(controller.release)
	headers := http.Header{}
	headers.Set("Authorization", "Bearer shared-token")
	headers.Set("Cookie", "session=cookie-token")
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?token=query-token"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("WebSocket connection failed: %v", err)
	}
	defer conn.Close()
	select {
	case <-interceptor.handshakeSeen:
	case <-time.After(time.Second):
		t.Fatal("optional handshake interceptor did not run before upgrade")
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("authenticate")); err != nil {
		t.Fatalf("message send failed: %v", err)
	}
	select {
	case <-interceptor.messageSeen:
	case <-time.After(time.Second):
		t.Fatal("common message interceptor did not run")
	}
	if interceptor.handshakeAuth != "Bearer shared-token" || interceptor.messageAuth != "Bearer shared-token" || interceptor.messageQuery != "query-token" || interceptor.messageCookie != "cookie-token" || interceptor.messageRemote == "" {
		t.Fatalf("credentials were not preserved across handshake/message contexts: %+v", interceptor)
	}
}

func newTestRuntime(t *testing.T, controller any, handler any, options boot.WebSocketOptions, handshake ...core.WebSocketHandshakeInterceptor) (*wsruntime.Runtime, wsruntime.Registration) {
	t.Helper()

	registry := wsruntime.NewRegistry()
	if err := registry.Register("/", handler); err != nil {
		t.Fatalf("WebSocket 등록 실패: %v", err)
	}
	registration := registry.Registrations()[0]

	c := container.New()
	switch typed := controller.(type) {
	case *cancellationController:
		_ = c.RegisterConstructor(func() *cancellationController { return typed })
	case *concurrentSendController:
		_ = c.RegisterConstructor(func() *concurrentSendController { return typed })
	case *stopWaitController:
		_ = c.RegisterConstructor(func() *stopWaitController { return typed })
	default:
		t.Fatalf("지원하지 않는 테스트 컨트롤러: %T", controller)
	}

	router := spinerouter.NewRouter()
	router.Register("WS", registration.Path, registration.Meta)
	p := pipeline.NewPipeline(router, invoker.NewInvoker(c))
	p.AddArgumentResolver(&resolver.StdContextResolver{})

	for _, interceptor := range handshake {
		if common, ok := interceptor.(core.Interceptor); ok {
			p.AddInterceptor(common)
		}
	}
	adapter, err := gorillaplatform.NewFactory().New(wsruntime.AdapterOptions(options))
	if err != nil {
		t.Fatal(err)
	}
	return wsruntime.NewRuntime(p, adapter, options, handshake...), registration
}

func newRuntimeTestServer(runtime *wsruntime.Runtime, registration wsruntime.Registration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		runtime.HandleConn(w, req, registration)
	}))
}

func dialRuntimeTestServer(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket 연결 실패: %v", err)
	}
	return conn
}
