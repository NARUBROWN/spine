package publish

import (
	"context"

	eventadapter "github.com/NARUBROWN/spine/adapter/event"
)

type EventPublisher interface {
	Publish(ctx context.Context, event eventadapter.Message) error
}
