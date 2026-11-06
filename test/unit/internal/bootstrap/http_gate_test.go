package bootstrap

import (
	"net/http"
	"sync"
)

// preparedHandler는 테스트 어댑터의 가장 바깥 native 요청을 추적합니다.
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
	if h.closing {
		return
	}
	h.closing = true
	if h.active == 0 {
		close(h.drained)
	}
}
