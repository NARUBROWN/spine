package ws

import (
	"context"
	"errors"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/ws/sendctx"
	pkgws "github.com/NARUBROWN/spine/pkg/ws"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestMessageInputLeaseAndPureState(t *testing.T) {
	request := newWebSocketRequestSnapshot(httptest.NewRequest("GET", "/ws?token=before", nil))
	state := execution.NewState(context.Background())
	for _, name := range []string{"Payload", "ConnID", "Path", "Method", "SetRoute", "EventName", "Ack", "Nack"} {
		if _, ok := reflect.TypeOf(state).MethodByName(name); ok {
			t.Fatalf("state owns transport method %s", name)
		}
	}
	input := execution.NewInput(execution.Sources{WebSocketRequest: request, ConnectionID: "conn", WebSocket: &execution.WebSocketMessage{Kind: pkgws.TextMessage, Payload: []byte("original")}})
	public := execution.NewContext(state, input)
	view, ok := public.WebSocket()
	if !ok {
		t.Fatal("missing view")
	}
	view.Payload()[0] = 'X'
	if string(view.Payload()) != "original" {
		t.Fatal("mutable input exposed")
	}
	if view.ConnID() != "conn" || view.Query("token") != "before" {
		t.Fatal("wrong session")
	}
	input.Close()
	if view.Payload() != nil || view.ConnID() != "" || view.Query("token") != "" {
		t.Fatal("input retained after completion")
	}
	if _, ok := public.WebSocket(); ok {
		t.Fatal("closed input available")
	}
}
func TestLegacySendCapabilityIsRevoked(t *testing.T) {
	calls := 0
	ctx, release := sendctx.Bind(context.Background(), func(int, []byte) error { calls++; return nil })
	retained := sendctx.From(ctx)
	if err := pkgws.Send(ctx, 1, []byte("pong")); err != nil || calls != 1 {
		t.Fatalf("send: %v", err)
	}
	release()
	if err := retained(1, nil); err == nil || calls != 1 {
		t.Fatal("retained callback still sends")
	}
	if err := pkgws.Send(ctx, 1, nil); !errors.Is(err, pkgws.ErrConnectionNotFound) {
		t.Fatal("retained context still sends")
	}
}
