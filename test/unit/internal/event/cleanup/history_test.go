package cleanup

import (
	"errors"
	"fmt"
	"github.com/NARUBROWN/spine/pkg/boot"
	"sync"
	"testing"
	"time"
)

func TestHistoryBoundsCountAndPreservesErrorIdentity(t *testing.T) {
	h := NewHistory(boot.CleanupErrorHistoryOptions{MaxEntries: 2, MaxAge: time.Hour})
	a, b, c := errors.New("old"), errors.New("second"), errors.New("last")
	h.Add(a)
	h.Add(b)
	h.Add(c)
	got := h.Take()
	if errors.Is(got, a) || !errors.Is(got, b) || !errors.Is(got, c) {
		t.Fatal(got)
	}
	if h.timer != nil || h.entries != nil {
		t.Fatal("final snapshot retained timer or buffer")
	}
}

func TestHistoryExpiresAtAgeBoundary(t *testing.T) {
	now := time.Now()
	h := NewHistory(boot.CleanupErrorHistoryOptions{MaxAge: time.Hour})
	h.now = func() time.Time { return now }
	h.Add(errors.New("expired"))
	now = now.Add(time.Hour)
	if got := h.Take(); got != nil {
		t.Fatal(got)
	}
}

func TestHistoryReleasesExpiredErrorsWhileIdle(t *testing.T) {
	h := NewHistory(boot.CleanupErrorHistoryOptions{MaxAge: 10 * time.Millisecond})
	defer h.Take()
	h.Add(errors.New("expires without Add or Take"))
	deadline := time.After(time.Second)
	for {
		h.mu.Lock()
		empty := len(h.entries) == 0
		h.mu.Unlock()
		if empty {
			return
		}
		select {
		case <-deadline:
			t.Fatal("idle history did not expire")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestHistoryDefaultsAndConcurrentCleanup(t *testing.T) {
	var h History
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); h.Add(fmt.Errorf("cleanup %d", i)) }(i)
	}
	wg.Wait()
	if h.policy.MaxEntries != boot.DefaultCleanupErrorHistoryMaxEntries || h.policy.MaxAge != boot.DefaultCleanupErrorHistoryMaxAge {
		t.Fatal(h.policy)
	}
	got := h.Take()
	if len(got.(interface{ Unwrap() []error }).Unwrap()) != 32 {
		t.Fatal(got)
	}
}
