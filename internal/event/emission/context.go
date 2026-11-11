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

type publisherView struct{ publisher Publisher }

func (v publisherView) Publish(events ...DomainEvent) {
	v.publisher.Publish(events...)
}

func WithPublisher(ctx context.Context, publisher Publisher) context.Context {
	if publisher == nil {
		return context.WithValue(ctx, publisherKey{}, nil)
	}
	return context.WithValue(ctx, publisherKey{}, publisherView{publisher: publisher})
}

func FromContext(ctx context.Context) Publisher {
	publisher, _ := ctx.Value(publisherKey{}).(Publisher)
	return publisher
}
