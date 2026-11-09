package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	wsadapter "github.com/NARUBROWN/spine/contract/websocket"
	"github.com/NARUBROWN/spine/core"
)

type cleanupPanicWSConnection struct {
	stage   string
	started chan struct{}
	closed  chan struct{}
	once    sync.Once
	closes  atomic.Int32
}

func (c *cleanupPanicWSConnection) ReadMessage() (int, []byte, error) {
	close(c.started)
	<-c.closed
	return 0, nil, errors.New("closed")
}
func (*cleanupPanicWSConnection) WriteMessage(int, []byte, time.Time) error { return nil }
func (*cleanupPanicWSConnection) Ping(time.Time) error                      { return nil }
func (c *cleanupPanicWSConnection) WriteClose(int, string, time.Time) error {
	if c.stage == "WriteClose" {
		panic("write shutdown panic")
	}
	return nil
}
func (c *cleanupPanicWSConnection) Close() error {
	first := c.closes.Add(1) == 1
	c.once.Do(func() { close(c.closed) })
	if first && c.stage == "Close" {
		panic("close shutdown panic")
	}
	return nil
}

type cleanupPanicWSAdapter struct{ conn *cleanupPanicWSConnection }

func (a cleanupPanicWSAdapter) Upgrade(http.ResponseWriter, *http.Request) (wsadapter.Connection, error) {
	return a.conn, nil
}

func TestApplicationCloseContinuesAfterWSWorkerPanic(t *testing.T) {
	for _, stage := range []string{"WriteClose", "Close"} {
		t.Run(stage, func(t *testing.T) {
			conn := &cleanupPanicWSConnection{stage: stage, started: make(chan struct{}), closed: make(chan struct{})}
			config := wsFactoryConfig(t, &testWSFactory{version: wsadapter.ContractVersion, instance: cleanupPanicWSAdapter{conn}})
			var stops, publishers atomic.Int32
			config.CustomTransports = []core.CustomTransport{callbackTransport{stop: func(context.Context) error { stops.Add(1); return nil }}}
			prepared, err := Prepare(context.Background(), config)
			if err != nil {
				t.Fatal(err)
			}
			prepared.publishers = append(prepared.publishers, publisherCloser{name: "regression", close: func(context.Context) error { publishers.Add(1); return nil }})
			handled := make(chan struct{})
			go func() {
				defer close(handled)
				prepared.wsRuntime.HandleConn(httptest.NewRecorder(), httptest.NewRequest("GET", "http://example.test/ws", nil), config.WebSocketRegistry.Registrations()[0])
			}()
			select {
			case <-conn.started:
			case <-time.After(time.Second):
				t.Fatal("connection was not activated")
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err = prepared.Close(ctx)
			if err == nil || !strings.Contains(err.Error(), "shutdown panic") {
				t.Fatalf("Close did not report the recovered panic: %v", err)
			}
			if stops.Load() != 1 || publishers.Load() != 1 {
				t.Fatalf("cleanup did not continue: transports=%d publishers=%d", stops.Load(), publishers.Load())
			}
			select {
			case <-handled:
			case <-time.After(time.Second):
				t.Fatal("connection Close was not attempted after panic")
			}
			if again := prepared.Close(ctx); again != err {
				t.Fatalf("Close result changed: %v", again)
			}
		})
	}
}
