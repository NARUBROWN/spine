package publish

import "github.com/NARUBROWN/spine/internal/event/emission"

type DefaultEventBus struct {
	events []emission.DomainEvent
}

func NewEventBus() *DefaultEventBus {
	return &DefaultEventBus{}
}

func (b *DefaultEventBus) Publish(events ...emission.DomainEvent) {
	b.events = append(b.events, events...)
}

func (b *DefaultEventBus) Drain() []emission.DomainEvent {
	if len(b.events) == 0 {
		return nil
	}

	ev := b.events
	b.events = nil
	return ev
}
