package ws

import (
	"context"
	"errors"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/ws/sendctx"
	pkgws "github.com/NARUBROWN/spine/pkg/ws"
	"reflect"
	"testing"
)

func TestCommonContextAndInputHaveNoWebSocketAPI(t *testing.T) {
	state := execution.NewState(context.Background())
	for _, typ := range []reflect.Type{reflect.TypeOf(state), reflect.TypeFor[execution.Sources](), reflect.TypeOf(execution.NewContext(state, nil))} {
		for _, name := range []string{"WebSocket", "WebSocketRequest", "ConnID", "Payload"} {
			if _, ok := typ.MethodByName(name); ok {
				t.Fatalf("%s exposes %s", typ, name)
			}
			if typ.Kind() == reflect.Struct {
				if _, ok := typ.FieldByName(name); ok {
					t.Fatalf("%s owns %s", typ, name)
				}
			}
		}
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
