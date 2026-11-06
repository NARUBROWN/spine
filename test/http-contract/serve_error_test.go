package contractprobe

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"

	contract "github.com/NARUBROWN/spine/contract/http"
)

type permanentAcceptErrorListener struct {
	net.Listener
	entered chan struct{}
	release chan struct{}
	err     error
	once    sync.Once
}

func (l *permanentAcceptErrorListener) Accept() (net.Conn, error) {
	l.once.Do(func() { close(l.entered) })
	<-l.release
	return nil, l.err
}

func TestStopAcceptingPreservesPermanentServeErrors(t *testing.T) {
	for _, engine := range engines {
		for _, stopping := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stopping=%v", engine.name, stopping), func(t *testing.T) {
				inner, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer inner.Close()
				sentinel := errors.New("permanent listener failure")
				listener := &permanentAcceptErrorListener{Listener: inner, entered: make(chan struct{}), release: make(chan struct{}), err: fmt.Errorf("Accept: %w", sentinel)}
				a := newAdapter(t, engine, func(contract.HTTPContext) error { return nil }, hooks{}, -1)
				done := make(chan error, 1)
				go func() { done <- a.Serve(listener) }()
				receive(t, listener.entered)
				if stopping {
					a.StopAccepting()
				}
				close(listener.release)
				if err := receive(t, done); !errors.Is(err, sentinel) {
					t.Fatalf("real listener failure was replaced: %v", err)
				}
			})
		}
	}
}
