package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"sync"

	eventadapter "github.com/NARUBROWN/spine/contract/event"
	"github.com/NARUBROWN/spine/pkg/event/publish"
)

type EventDispatcher interface {
	Dispatch(ctx context.Context, events []publish.DomainEvent) error
}

type DefaultEventDispatcher struct {
	publishers []EventPublisher
}

func NewDefaultEventDispatcher(publishers ...EventPublisher) (*DefaultEventDispatcher, error) {
	for i, publisher := range publishers {
		if isNilEventPublisher(publisher) {
			return nil, fmt.Errorf("event publisher[%d] cannot be nil", i)
		}
	}

	return &DefaultEventDispatcher{
		publishers: append([]EventPublisher(nil), publishers...),
	}, nil
}

func isNilEventPublisher(publisher EventPublisher) bool {
	if publisher == nil {
		return true
	}

	value := reflect.ValueOf(publisher)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (d *DefaultEventDispatcher) Dispatch(ctx context.Context, events []publish.DomainEvent) error {
	var dispatchErrs []error

	for _, e := range events {
		eventName := e.Name()
		payload, err := json.Marshal(e)
		if err != nil {
			dispatchErrs = append(dispatchErrs, fmt.Errorf("serialize event (%s): %w", eventName, err))
			continue
		}
		message := eventadapter.Message{EventName: eventName, OccurredAt: e.OccurredAt(), Payload: payload}
		var wg sync.WaitGroup
		publisherErrs := make([]error, len(d.publishers))
		for i, p := range d.publishers {
			wg.Add(1)
			go func(index int, p EventPublisher) {
				defer wg.Done()
				defer func() {
					if recovered := recover(); recovered != nil {
						publisherErrs[index] = publisherPanicError(index, eventName, recovered)
						log.Printf("[EventDispatcher] Publisher panic recovered (%s, publisher=%d): %v", eventName, index, recovered)
					}
				}()
				if err := p.Publish(ctx, message); err != nil {
					log.Printf("[EventDispatcher] Failed to publish event (%s): %v", eventName, err)
					publisherErrs[index] = fmt.Errorf("failed to publish event (%s): %w", eventName, err)
				}
			}(i, p)
		}
		wg.Wait()

		for _, err := range publisherErrs {
			if err != nil {
				dispatchErrs = append(dispatchErrs, err)
			}
		}
	}

	return errors.Join(dispatchErrs...)
}

func publisherPanicError(index int, eventName string, recovered any) error {
	if err, ok := recovered.(error); ok {
		return fmt.Errorf("publisher[%d] panicked while publishing event (%s): %w", index, eventName, err)
	}
	return fmt.Errorf("publisher[%d] panicked while publishing event (%s): %v", index, eventName, recovered)
}
