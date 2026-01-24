package consumer

import "context"

type EventHandler func(
	ctx context.Context,
	eventName string,
	payload []byte,
) error
