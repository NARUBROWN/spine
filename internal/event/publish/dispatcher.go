package publish

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/NARUBROWN/spine/pkg/event/publish"
)

type EventDispatcher interface {
	Dispatch(ctx context.Context, events []publish.DomainEvent) error
}

type DefaultEventDispatcher struct {
	Publishers []EventPublisher
}

func (d *DefaultEventDispatcher) Dispatch(ctx context.Context, events []publish.DomainEvent) error {
	var dispatchErrs []error

	for _, e := range events {
		if len(d.Publishers) == 1 {
			if err := d.Publishers[0].Publish(ctx, e); err != nil {
				log.Printf("[EventDispatcher] 이벤트 발행 실패 (%s): %v", e.Name(), err)
				dispatchErrs = append(dispatchErrs, fmt.Errorf("이벤트 발행 실패 (%s): %w", e.Name(), err))
			}
			continue
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, p := range d.Publishers {
			wg.Add(1)
			go func(p EventPublisher) {
				defer wg.Done()
				if err := p.Publish(ctx, e); err != nil {
					log.Printf("[EventDispatcher] 이벤트 발행 실패 (%s): %v", e.Name(), err)
					mu.Lock()
					dispatchErrs = append(dispatchErrs, fmt.Errorf("이벤트 발행 실패 (%s): %w", e.Name(), err))
					mu.Unlock()
				}
			}(p)
		}
		wg.Wait()
	}

	return errors.Join(dispatchErrs...)
}
