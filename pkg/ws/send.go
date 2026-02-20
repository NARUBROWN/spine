package ws

import "context"

type senderKeyType struct{}

var SenderKey = senderKeyType{}

type Sender interface {
	Send(messageType int, data []byte) error
}

func Send(ctx context.Context, messageType int, data []byte) error {
	sender, ok := ctx.Value(SenderKey).(Sender)
	if !ok || sender == nil {
		return nil
	}
	return sender.Send(messageType, data)
}
