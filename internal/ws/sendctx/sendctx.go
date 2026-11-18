package sendctx

import (
	"context"
	"errors"
	"sync"
)

type senderKey struct{}

var ErrUnavailable = errors.New("websocket execution sender is no longer available")

type slot struct {
	mu   sync.RWMutex
	send func(int, []byte) error
}

// Bind is a temporary compatibility capability, revoked after one execution.
func Bind(ctx context.Context, send func(int, []byte) error) (context.Context, func()) {
	s := &slot{send: send}
	return context.WithValue(ctx, senderKey{}, s), func() { s.mu.Lock(); s.send = nil; s.mu.Unlock() }
}
func From(ctx context.Context) func(int, []byte) error {
	s, _ := ctx.Value(senderKey{}).(*slot)
	if s == nil {
		return nil
	}
	return func(kind int, payload []byte) error {
		s.mu.RLock()
		defer s.mu.RUnlock()
		if s.send == nil {
			return ErrUnavailable
		}
		return s.send(kind, payload)
	}
}
