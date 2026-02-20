package ws

import (
	"context"
	"testing"

	internalpublish "github.com/NARUBROWN/spine/internal/event/publish"
	pkgws "github.com/NARUBROWN/spine/pkg/ws"
)

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
