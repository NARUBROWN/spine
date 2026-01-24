package publish

import (
	"context"

	"github.com/NARUBROWN/spine/pkg/event/publish"
)

type EventDispatcher interface {
	Dispatch(ctx context.Context, events []publish.DomainEvent)
}

type DefaultEventDispatcher struct {
	Publishers []EventPublisher
}

func (d *DefaultEventDispatcher) Dispatch(ctx context.Context, events []publish.DomainEvent) {
	for _, e := range events {
		for _, p := range d.Publishers {
			_ = p.Publish(ctx, e)
		}
	}
}
