package ws

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/NARUBROWN/spine/internal/resolver"
	spinerouter "github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/pkg/boot"
	pkgws "github.com/NARUBROWN/spine/pkg/ws"
	"github.com/gorilla/websocket"
)

type cancellationController struct {
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
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
		t.Fatal("Runtime.Stop이 활성 핸들러의 context를 취소하지 않았습니다")
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
	for err := range controller.errs {
		if err != nil {
			t.Fatalf("동시 전송 실패: %v", err)
		}
	}
}

func newTestRuntime(t *testing.T, controller any, handler any, options boot.WebSocketOptions) (*Runtime, Registration) {
	t.Helper()

	registry := NewRegistry()
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
	default:
		t.Fatalf("지원하지 않는 테스트 컨트롤러: %T", controller)
	}

	router := spinerouter.NewRouter()
	router.Register("WS", registration.Path, registration.Meta)
	p := pipeline.NewPipeline(router, invoker.NewInvoker(c))
	p.AddArgumentResolver(&resolver.StdContextResolver{})

	return NewRuntime(registry, p, options), registration
}

func newRuntimeTestServer(runtime *Runtime, registration Registration) *httptest.Server {
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
