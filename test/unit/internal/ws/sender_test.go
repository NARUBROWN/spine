package ws

import (
	"context"
	"errors"
	contract "github.com/NARUBROWN/spine/contract/websocket"
	public "github.com/NARUBROWN/spine/pkg/ws"
	"sync"
	"testing"
	"time"
)

type senderProbe struct {
	contract.Connection
	write func(int, []byte, time.Time) error
}

func (c *senderProbe) WriteMessage(kind int, data []byte, deadline time.Time) error {
	return c.write(kind, data, deadline)
}

func TestSenderCancellationWhileWaitingDoesNotCloseSharedConnection(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	native := &senderProbe{write: func(_ int, _ []byte, deadline time.Time) error {
		once.Do(func() { close(started); <-release })
		return nil
	}}
	sender := NewSender()
	sender.add("one", &trackedConn{conn: native})
	done := make(chan error, 1)
	go func() { done <- sender.Send(t.Context(), "one", 1, []byte("first")) }()
	<-started
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := sender.Send(ctx, "one", 1, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(t.Context(), "one", 1, nil); err != nil {
		t.Fatal(err)
	}
}
func TestSenderUsesCallerDeadlineAndForwardsWriteFailure(t *testing.T) {
	failure := errors.New("write failed")
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	want, _ := ctx.Deadline()
	sender := NewSender()
	sender.add("one", &trackedConn{conn: &senderProbe{write: func(kind int, payload []byte, deadline time.Time) error {
		if !deadline.Equal(want) || kind != public.BinaryMessage || string(payload) != "body" {
			t.Errorf("deadline=%v kind=%d payload=%s", deadline, kind, payload)
		}
		return failure
	}}})
	if err := sender.Send(ctx, "one", public.BinaryMessage, []byte("body")); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	sender.remove("one")
	if err := sender.Send(ctx, "one", 1, nil); !errors.Is(err, public.ErrConnectionNotFound) {
		t.Fatal(err)
	}
}
func BenchmarkSenderConnectionChurn(b *testing.B) {
	sender := NewSender()
	conn := &trackedConn{conn: &senderProbe{write: func(int, []byte, time.Time) error { return nil }}}
	for b.Loop() {
		sender.add("one", conn)
		if err := sender.Send(context.Background(), "one", 1, nil); err != nil {
			b.Fatal(err)
		}
		sender.remove("one")
	}
	if len(sender.connections) != 0 {
		b.Fatal("retained closed connections")
	}
	sender.Close()
}
