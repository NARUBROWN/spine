package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	adapter "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/event/consumer"
	"github.com/NARUBROWN/spine/internal/ws"
)

var ErrInvalidContext = errors.New("context must not be nil")

type consumerRuntime struct {
	name    string
	runtime *consumer.Runtime
}

type publisherCloser struct {
	name  string
	close func(context.Context) error
}

// Application은 준비가 성공한 자원만 보관하고 공통 정리 순서를 관리합니다.
type Application struct {
	config     Config
	server     adapter.HTTPAdapter
	handler    *preparedHandler
	listener   *managedHTTPListener
	wsRuntime  *ws.Runtime
	consumers  []consumerRuntime
	transports []core.CustomTransport
	publishers []publisherCloser
	closeOnce  sync.Once
	closeDone  chan struct{}
	closeErr   error
}

func (a *Application) Handler() http.Handler {
	if a.handler == nil {
		return nil
	}
	return a.handler
}

func (a *Application) shutdownTimeout() time.Duration {
	if a.config.ShutdownTimeout > 0 {
		return a.config.ShutdownTimeout
	}
	return 10 * time.Second
}

func (a *Application) closeWithTimeout() error {
	ctx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout())
	defer cancel()
	return a.Close(ctx)
}

// Close는 정리를 한 번 시작하고 각 호출의 context로 완료를 기다립니다.
// 첫 호출의 취소를 정리 작업에 전파하지 않아 진행 중인 요청의 자원을 보존합니다.
func (a *Application) Close(ctx context.Context) error {
	if isNilHTTPValue(ctx) {
		return ErrInvalidContext
	}
	a.closeOnce.Do(func() {
		if a.handler != nil {
			a.handler.stop()
		}
		go a.closeResources()
	})
	select {
	case <-a.closeDone:
		return a.closeErr
	default:
	}
	select {
	case <-a.closeDone:
		return a.closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Application) closeResources() {
	defer close(a.closeDone)
	step := func(name string, close func() error) {
		if err := closeResource(name, close); err != nil {
			a.closeErr = errors.Join(a.closeErr, err)
			log.Printf("[Bootstrap] %v", err)
		}
	}
	if a.wsRuntime != nil {
		step("WebSocket runtime", func() error { a.wsRuntime.Stop(); return nil })
	}
	if a.server != nil {
		step("HTTP adapter", func() error { return a.server.Shutdown(context.Background()) })
	}
	if a.listener != nil {
		step("HTTP listener", a.listener.Close)
	}
	if a.handler != nil {
		<-a.handler.drained
	}
	for i := len(a.consumers) - 1; i >= 0; i-- {
		runtime := a.consumers[i]
		step(runtime.name+" consumer", runtime.runtime.Stop)
	}
	ctx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout())
	defer cancel()
	for i := len(a.transports) - 1; i >= 0; i-- {
		transport := a.transports[i]
		step("custom transport", func() error { return transport.Stop(ctx) })
	}
	for i := len(a.publishers) - 1; i >= 0; i-- {
		publisher := a.publishers[i]
		step(publisher.name+" publisher", func() error { return publisher.close(context.Background()) })
	}
}

// 정리 콜백 하나의 패닉이 다른 자원 회수를 생략하거나 소유 고루틴을 중단하지 않게 합니다.
func closeResource(name string, close func() error) (err error) {
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("%s cleanup panicked: %v", name, value)
		}
	}()
	if err = close(); err != nil {
		return fmt.Errorf("%s cleanup failed: %w", name, err)
	}
	return nil
}

// preparedHandler는 외부 서버에서 실행하는 요청도 자원 정리 전에 끝나도록 추적합니다.
// ResponseWriter를 그대로 전달하여 업그레이드에 필요한 native 인터페이스를 보존합니다.
type preparedHandler struct {
	native  http.Handler
	mu      sync.Mutex
	closing bool
	active  int
	drained chan struct{}
}

func newPreparedHandler(native http.Handler) *preparedHandler {
	return &preparedHandler{native: native, drained: make(chan struct{})}
}

func (h *preparedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	if h.closing {
		h.mu.Unlock()
		http.Error(w, "application is closing", http.StatusServiceUnavailable)
		return
	}
	h.active++
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.active--
		if h.closing && h.active == 0 {
			close(h.drained)
		}
		h.mu.Unlock()
	}()
	h.native.ServeHTTP(w, r)
}

func (h *preparedHandler) stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closing = true
	if h.active == 0 {
		close(h.drained)
	}
}
