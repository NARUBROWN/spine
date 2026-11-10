package emission

import (
	"context"
	"time"
)

type DomainEvent interface {
	Name() string
	OccurredAt() time.Time
}

type Publisher interface {
	Publish(events ...DomainEvent)
}

type Bus interface {
	Publisher
	Drain() []DomainEvent
}

type publisherKey struct{}

func WithPublisher(ctx context.Context, publisher Publisher) context.Context {
	return context.WithValue(ctx, publisherKey{}, publisher)
}

func FromContext(ctx context.Context) Publisher {
	publisher, _ := ctx.Value(publisherKey{}).(Publisher)
	return publisher
}
