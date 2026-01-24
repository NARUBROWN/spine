package publish

import (
	"context"
)

type EventPublisher interface {
	Publish(events ...DomainEvent)
}

type publisherKeyType struct{}

var PublisherKey = publisherKeyType{}

func Event(ctx context.Context, events ...DomainEvent) {
	bus, ok := ctx.Value(PublisherKey).(EventPublisher)
	if !ok || bus == nil {
		return
	}
	bus.Publish(events...)
}
