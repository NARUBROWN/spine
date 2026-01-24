package publish

import "github.com/NARUBROWN/spine/pkg/event/publish"

type EventBus interface {
	Publish(events ...publish.DomainEvent)
	Drain() []publish.DomainEvent
}
