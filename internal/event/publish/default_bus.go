package publish

import "github.com/NARUBROWN/spine/pkg/event/publish"

type DefaultEventBus struct {
	events []publish.DomainEvent
}

func NewEventBus() *DefaultEventBus {
	return &DefaultEventBus{}
}

func (b *DefaultEventBus) Publish(events ...publish.DomainEvent) {
	b.events = append(b.events, events...)
}

func (b *DefaultEventBus) Drain() []publish.DomainEvent {
	if len(b.events) == 0 {
		return nil
	}

	ev := b.events
	b.events = nil
	return ev
}
