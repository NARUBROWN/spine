package extract

import "github.com/NARUBROWN/spine/pkg/event/publish"

type EventExtractor interface {
	Extract(results []any) []publish.DomainEvent
}

type DefaultEventExtractor struct{}

func (e DefaultEventExtractor) Extract(results []any) []publish.DomainEvent {
	var events []publish.DomainEvent

	for _, r := range results {
		switch v := r.(type) {
		case publish.DomainEvent:
			events = append(events, v)
		case []publish.DomainEvent:
			events = append(events, v...)
		}
	}

	return events
}
