package ws

import (
	"context"
	"errors"

	"github.com/NARUBROWN/spine/internal/ws/sendctx"
)

// Sender is an application-owned connection-addressed transmitter supplied by DI.
// Send waits for the platform write. It does not establish connections or imply
// receipt by the peer. The write deadline is bounded by ctx and server policy.
type Sender interface {
	Send(ctx context.Context, connectionID string, messageType int, data []byte) error
}

var ErrConnectionNotFound = errors.New("WebSocket connection is not available")
var ErrSenderClosed = errors.New("WebSocket sender is closed")

// Send writes to the connection bound to ctx.
// Deprecated: inject Sender and address a connection explicitly.
func Send(ctx context.Context, messageType int, data []byte) error {
	send := sendctx.From(ctx)
	if send == nil {
		return ErrConnectionNotFound
	}
	return send(messageType, data)
}
