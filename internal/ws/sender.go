package ws

import (
	"context"
	"errors"
	public "github.com/NARUBROWN/spine/pkg/ws"
	"sync"
	"time"
)

// Sender exposes transmission without exposing runtime lifecycle or native sockets.
type Sender struct {
	mu          sync.RWMutex
	connections map[string]*trackedConn
	timeout     time.Duration
	closed      bool
}

func NewSender() *Sender {
	return &Sender{connections: make(map[string]*trackedConn), timeout: defaultWSWriteTimeout}
}
func (s *Sender) Send(ctx context.Context, id string, kind int, data []byte) error {
	if ctx == nil {
		return errors.New("WebSocket send context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if kind != public.TextMessage && kind != public.BinaryMessage {
		return errors.New("WebSocket Send accepts text or binary messages only")
	}
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return public.ErrSenderClosed
	}
	conn, timeout := s.connections[id], s.timeout
	s.mu.RUnlock()
	if conn == nil {
		return public.ErrConnectionNotFound
	}
	return conn.writeMessageContext(ctx, kind, data, timeout)
}
func (s *Sender) add(id string, conn *trackedConn) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.connections[id] = conn
	}
}
func (s *Sender) remove(id string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.connections, id)
	s.mu.Unlock()
}
func (s *Sender) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.closed = true
	clear(s.connections)
	s.mu.Unlock()
}
