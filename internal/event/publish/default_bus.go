package publish

import (
	"sync"

	"github.com/NARUBROWN/spine/internal/event/emission"
)

type DefaultEventBus struct {
	mu     sync.Mutex
	events []emission.DomainEvent
}

func NewEventBus() *DefaultEventBus {
	return &DefaultEventBus{}
}

func (b *DefaultEventBus) Publish(events ...emission.DomainEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, events...)
}

func (b *DefaultEventBus) Drain() []emission.DomainEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.events) == 0 {
		return nil
	}

	ev := b.events
	b.events = nil
	return ev
}
