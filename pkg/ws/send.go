package ws

import (
	"context"

	"github.com/NARUBROWN/spine/internal/ws/sendctx"
)

type Sender interface {
	Send(messageType int, data []byte) error
}

func Send(ctx context.Context, messageType int, data []byte) error {
	send := sendctx.From(ctx)
	if send == nil {
		return nil
	}
	return send(messageType, data)
}
