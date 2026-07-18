package publish

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"sync"

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
		var wg sync.WaitGroup
		publisherErrs := make([]error, len(d.publishers))
		for i, p := range d.publishers {
			wg.Add(1)
			go func(index int, p EventPublisher) {
				defer wg.Done()
				if err := p.Publish(ctx, e); err != nil {
					log.Printf("[EventDispatcher] Failed to publish event (%s): %v", e.Name(), err)
					publisherErrs[index] = fmt.Errorf("failed to publish event (%s): %w", e.Name(), err)
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
