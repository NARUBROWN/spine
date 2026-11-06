// Package cleanup은 이벤트 자원 정리 오류의 보관 건수와 수명을 제한합니다.
package cleanup

import (
	"errors"
	"sync"
	"time"

	"github.com/NARUBROWN/spine/pkg/boot"
)

type record struct {
	at  time.Time
	err error
}

// History의 zero value는 기본 정책을 사용합니다. 사용을 시작한 뒤 복사하면 안 됩니다.
type History struct {
	mu      sync.Mutex
	policy  boot.CleanupErrorHistoryOptions
	entries []record
	timer   *time.Timer
	now     func() time.Time
	closed  bool
}

func NewHistory(options ...boot.CleanupErrorHistoryOptions) History {
	var policy boot.CleanupErrorHistoryOptions
	if len(options) > 0 {
		policy = options[0]
	}
	return History{policy: policy}
}

func (h *History) defaults() {
	if h.policy.MaxEntries <= 0 {
		h.policy.MaxEntries = boot.DefaultCleanupErrorHistoryMaxEntries
	}
	if h.policy.MaxAge <= 0 {
		h.policy.MaxAge = boot.DefaultCleanupErrorHistoryMaxAge
	}
	if h.now == nil {
		h.now = time.Now
	}
}

func (h *History) prune(now time.Time) {
	cutoff := now.Add(-h.policy.MaxAge)
	n := 0
	for n < len(h.entries) && !h.entries[n].at.After(cutoff) {
		n++
	}
	if n > 0 {
		remaining := copy(h.entries, h.entries[n:])
		clear(h.entries[remaining:])
		h.entries = h.entries[:remaining]
	}
}

func (h *History) schedule() {
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
	if h.closed || len(h.entries) == 0 {
		return
	}
	delay := h.entries[0].at.Add(h.policy.MaxAge).Sub(h.now())
	h.timer = time.AfterFunc(max(delay, 0), h.expire)
}

func (h *History) expire() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.prune(h.now())
	h.schedule()
}

func (h *History) Add(err error) {
	if err == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.defaults()
	now := h.now()
	h.prune(now)
	if len(h.entries) == h.policy.MaxEntries {
		copy(h.entries, h.entries[1:])
		h.entries = h.entries[:len(h.entries)-1]
	}
	h.entries = append(h.entries, record{now, err})
	h.schedule()
}

// Take는 만료되지 않은 오류의 종료 스냅샷을 반환하고 타이머와 보관 버퍼를 해제합니다.
func (h *History) Take() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.defaults()
	h.prune(h.now())
	errs := make([]error, len(h.entries))
	for i, entry := range h.entries {
		errs[i] = entry.err
	}
	h.closed = true
	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}
	h.entries = nil
	return errors.Join(errs...)
}
