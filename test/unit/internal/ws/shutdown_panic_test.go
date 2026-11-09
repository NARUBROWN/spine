package ws

import (
	"context"
	"errors"
	"testing"
	"time"
)

type shutdownPanicConnection struct {
	closeOnlyConnection
	writePanic any
	closePanic any
}

func (c *shutdownPanicConnection) WriteClose(int, string, time.Time) error {
	if c.writePanic != nil {
		panic(c.writePanic)
	}
	return nil
}
func (c *shutdownPanicConnection) Close() error {
	c.closes++
	if c.closePanic != nil {
		panic(c.closePanic)
	}
	return nil
}

func TestRuntimeStopRecoversWorkerPanicsAtCallerBoundary(t *testing.T) {
	for _, stage := range []string{"WriteClose", "Close", "both"} {
		t.Run(stage, func(t *testing.T) {
			cause := errors.New("adapter cleanup panic")
			broken, healthy := &shutdownPanicConnection{}, &shutdownPanicConnection{}
			if stage != "Close" {
				broken.writePanic = cause
			}
			if stage != "WriteClose" {
				broken.closePanic = cause
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runtime := &Runtime{ctx: ctx, cancel: cancel, options: normalizedWebSocketOptions{WriteTimeout: time.Second}, conns: map[string]*trackedConn{
				"broken": {conn: broken}, "healthy": {conn: healthy},
			}}
			var recovered any
			func() { defer func() { recovered = recover() }(); runtime.Stop() }()
			if recovered != cause {
				t.Fatalf("caller must recover the original cleanup panic: %v", recovered)
			}
			if broken.closes != 1 || healthy.closes != 1 {
				t.Fatalf("every Close must be attempted even after WriteClose panic: broken=%d healthy=%d", broken.closes, healthy.closes)
			}
		})
	}
}
