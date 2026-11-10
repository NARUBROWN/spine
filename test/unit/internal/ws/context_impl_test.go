package ws

import (
	"context"
	"sync"
	"testing"
	"time"

	internalpublish "github.com/NARUBROWN/spine/internal/event/publish"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	pkgws "github.com/NARUBROWN/spine/pkg/ws"
)

type mutableCookiesHandshakeContext struct {
	*webSocketRequestSnapshot
	cookies map[string]string
}

func (c *mutableCookiesHandshakeContext) Cookies() map[string]string {
	return c.cookies
}

func TestWSExecutionContext_StoresAndExposesValues(t *testing.T) {
	ctx := NewWSExecutionContext(
		context.Background(),
		"conn-1",
		"/ws/echo",
		pkgws.TextMessage,
		[]byte(`{"message":"hello"}`),
		internalpublish.NewEventBus(),
		func(int, []byte) error { return nil },
	)

	if ctx.ConnID() != "conn-1" {
		t.Fatalf("connID가 잘못되었습니다: %s", ctx.ConnID())
	}
	if ctx.MessageType() != pkgws.TextMessage {
		t.Fatalf("message type이 잘못되었습니다: %d", ctx.MessageType())
	}
	if string(ctx.Payload()) != `{"message":"hello"}` {
		t.Fatalf("payload가 잘못되었습니다: %s", string(ctx.Payload()))
	}
	if ctx.Method() != "WS" {
		t.Fatalf("메서드가 잘못되었습니다: %s", ctx.Method())
	}
	if ctx.Path() != "/ws/echo" {
		t.Fatalf("path가 잘못되었습니다: %s", ctx.Path())
	}

	ctx.Set("k", "v")
	v, ok := ctx.Get("k")
	if !ok || v != "v" {
		t.Fatalf("store 동작이 잘못되었습니다: %v, %v", v, ok)
	}
}

func TestWSExecutionContext_ContextProvidesSender(t *testing.T) {
	var gotType int
	var gotPayload []byte

	ctx := NewWSExecutionContext(
		context.Background(),
		"conn-1",
		"/ws/echo",
		pkgws.TextMessage,
		nil,
		internalpublish.NewEventBus(),
		func(messageType int, data []byte) error {
			gotType = messageType
			gotPayload = append([]byte(nil), data...)
			return nil
		},
	)

	err := pkgws.Send(ctx.Context(), pkgws.TextMessage, []byte("pong"))
	if err != nil {
		t.Fatalf("ws send 실패: %v", err)
	}
	if gotType != pkgws.TextMessage {
		t.Fatalf("전송 messageType이 잘못되었습니다: %d", gotType)
	}
	if string(gotPayload) != "pong" {
		t.Fatalf("전송 payload가 잘못되었습니다: %s", string(gotPayload))
	}
}

func TestWSExecutionContext_ExposesRouterPathParams(t *testing.T) {
	ctx := NewWSExecutionContext(
		context.Background(),
		"conn-1",
		"/ws/rooms/7",
		pkgws.TextMessage,
		nil,
		internalpublish.NewEventBus(),
		func(int, []byte) error { return nil },
	)

	ctx.SetRoute(map[string]string{"roomId": "7"}, []string{"roomId"})

	if got := ctx.Params(); got["roomId"] != "7" {
		t.Fatalf("path params가 노출되어야 합니다: %v", got)
	}
	keys := ctx.PathKeys()
	if len(keys) != 1 || keys[0] != "roomId" {
		t.Fatalf("path keys가 잘못되었습니다: %v", keys)
	}
}

func TestWSExecutionContext_EventBusIsSafeForConcurrentAccess(t *testing.T) {
	ctx := NewWSExecutionContext(
		context.Background(),
		"conn-1",
		"/ws/echo",
		pkgws.TextMessage,
		nil,
		nil,
		func(int, []byte) error { return nil },
	)

	const goroutines = 32
	buses := make([]any, goroutines)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			buses[index] = ctx.EventBus()
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 1; i < len(buses); i++ {
		if buses[i] != buses[0] {
			t.Fatal("동시 접근에서도 동일한 EventBus 인스턴스를 반환해야 합니다")
		}
	}
}

func TestWSExecutionContext_ClonesCookiesFromCustomHandshakeContext(t *testing.T) {
	request := &mutableCookiesHandshakeContext{
		webSocketRequestSnapshot: newWebSocketRequestSnapshot(nil),
		cookies:                  map[string]string{"session": "original"},
	}
	ctx := NewWSExecutionContext(
		context.Background(),
		"conn-1",
		"/ws/echo",
		pkgws.TextMessage,
		nil,
		nil,
		func(int, []byte) error { return nil },
		request,
	)
	request.cookies["session"] = "changed"

	messageCtx, ok := ctx.(execution.WebSocketRequestContext)
	if !ok {
		t.Fatal("WebSocket message context 계약을 구현해야 합니다")
	}
	if cookie, exists := messageCtx.Cookie("session"); !exists || cookie != "original" {
		t.Fatalf("handshake cookie snapshot이 변경됐습니다: value=%q exists=%v", cookie, exists)
	}
}

func TestInterceptorMessageViewCopiesPayloadAndHidesExecution(t *testing.T) {
	ctx := NewWSExecutionContext(context.Background(), "conn-1", "/ws/echo", pkgws.TextMessage,
		[]byte("original"), nil, func(int, []byte) error { return nil })
	public, ok := execution.NewContext(ctx).WebSocket()
	if !ok {
		t.Fatal("WebSocket 메시지 뷰가 없습니다")
	}
	if _, ok := public.(execution.Context); ok {
		t.Fatal("공개 메시지 뷰가 내부 실행 계약을 구현합니다")
	}
	if _, ok := public.(interface{ Set(string, any) }); ok {
		t.Fatal("WebSocket 뷰가 값 저장소를 노출합니다")
	}
	public.Payload()[0] = 'X'
	if got := string(ctx.Payload()); got != "original" {
		t.Fatalf("공개 뷰를 통해 원본 페이로드가 변경되었습니다: %s", got)
	}
}

type viewEvent struct{}

func (viewEvent) Name() string          { return "message.received" }
func (viewEvent) OccurredAt() time.Time { return time.Time{} }

func TestInterceptorViewPublishesThroughGoContext(t *testing.T) {
	ctx := NewWSExecutionContext(context.Background(), "conn-1", "/ws/echo", pkgws.TextMessage,
		nil, nil, func(int, []byte) error { return nil })
	publish.Event(execution.NewContext(ctx).ExecutionContext().Context(), viewEvent{})
	events := ctx.EventBus().Drain()
	if len(events) != 1 || events[0].Name() != "message.received" {
		t.Fatalf("이벤트가 내부 버스에 전달되지 않았습니다: %v", events)
	}
}
