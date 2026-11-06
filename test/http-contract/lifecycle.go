// Package contractprobe는 HTTP v3 적합성 검사용 최소 구현입니다.
// 제품 플랫폼, 커널 pipeline 또는 공개 설치 조합을 구현하지 않습니다.
package contractprobe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"

	contract "github.com/NARUBROWN/spine/contract/http"
)

type lifecycle struct {
	mu       sync.Mutex
	active   int
	stopping bool
	served   bool
	listener net.Listener
	drained  chan struct{}
	started  chan struct{}
	done     chan struct{}
	once     sync.Once
	err      error
}

func newLifecycle() *lifecycle {
	return &lifecycle{drained: make(chan struct{}), started: make(chan struct{}), done: make(chan struct{})}
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

func (l *lifecycle) claim(listener net.Listener) error {
	if isNil(listener) {
		return fmt.Errorf("%w: nil listener", contract.ErrInvalidState)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.served || l.stopping {
		return errors.Join(contract.ErrInvalidState, listener.Close())
	}
	l.served = true
	l.listener = listener
	close(l.started)
	return nil
}

func (l *lifecycle) enter() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopping {
		return false
	}
	l.active++
	return true
}

func (l *lifecycle) leave() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.active--
	if l.stopping && l.active == 0 {
		close(l.drained)
	}
}

func (l *lifecycle) StopAccepting() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopping {
		return
	}
	l.stopping = true
	if l.active == 0 {
		close(l.drained)
	}
}

func (l *lifecycle) isStopping() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.stopping
}

func (l *lifecycle) shutdown(ctx context.Context, stopNative func() error) error {
	if isNil(ctx) {
		return fmt.Errorf("%w: nil shutdown context", contract.ErrInvalidState)
	}
	l.StopAccepting()
	l.once.Do(func() {
		go func() {
			l.err = stopNative()
			<-l.drained
			close(l.done)
		}()
	})
	select {
	case <-l.done:
		return l.err
	default:
	}
	select {
	case <-l.done:
		return l.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
