package publish

import (
	"context"

	eventadapter "github.com/NARUBROWN/spine/contract/event"
)

type EventPublisher interface {
	Publish(ctx context.Context, event eventadapter.Message) error
}
