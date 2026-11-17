package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"sync"
	"time"

	eventadapter "github.com/NARUBROWN/spine/contract/event"
	public "github.com/NARUBROWN/spine/pkg/event/publish"
)

// Binding identifies the platform responsible for each transmission result.
type Binding struct {
	Name      string
	Publisher EventPublisher
}

type Publisher struct {
	bindings  []Binding
	mu        sync.Mutex
	closing   bool
	active    sync.WaitGroup
	closeOnce sync.Once
	closeDone chan struct{}
}

func NewPublisher(bindings ...Binding) (*Publisher, error) {
	for i, binding := range bindings {
		if isNilEventPublisher(binding.Publisher) {
			return nil, fmt.Errorf("event publisher[%d] cannot be nil", i)
		}
	}
	return &Publisher{bindings: append([]Binding(nil), bindings...), closeDone: make(chan struct{})}, nil
}

func isNilEventPublisher(publisher EventPublisher) bool {
	if publisher == nil {
		return true
	}
	v := reflect.ValueOf(publisher)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// Event serializes once and waits for all platforms, including on partial failure.
// Failures are joined in registration order and retain their original identity.
func (p *Publisher) Event(ctx context.Context, destination string, fields public.Fields, options ...public.Option) error {
	if ctx == nil {
		return errors.New("event context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(destination) == "" {
		return errors.New("event destination is empty")
	}
	p.mu.Lock()
	if p.closing {
		p.mu.Unlock()
		return eventadapter.ErrInvalidState
	}
	p.active.Add(1)
	p.mu.Unlock()
	defer p.active.Done()
	if len(p.bindings) == 0 {
		return public.ErrUnavailable
	}
	settings := public.Options{}
	for _, option := range options {
		if option != nil {
			option(&settings)
		}
	}
	metadata := maps.Clone(settings.Metadata)
	if settings.MessageID != "" {
		if metadata == nil {
			metadata = make(map[string]string)
		}
		metadata["message_id"] = settings.MessageID
	}
	at := settings.OccurredAt
	if at.IsZero() {
		at = time.Now()
	}
	if fields == nil {
		fields = public.Fields{}
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("serialize event (%s): %w", destination, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	message := eventadapter.Message{EventName: destination, OccurredAt: at, Payload: payload, Metadata: metadata}
	results := make([]error, len(p.bindings))
	var wg sync.WaitGroup
	for index, binding := range p.bindings {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					if cause, ok := recovered.(error); ok {
						results[index] = fmt.Errorf("%s publisher panicked (%s): %w", binding.Name, destination, cause)
					} else {
						results[index] = fmt.Errorf("%s publisher panicked (%s): %v", binding.Name, destination, recovered)
					}
				}
			}()
			if err := binding.Publisher.Publish(ctx, message); err != nil {
				results[index] = fmt.Errorf("%s publish (%s): %w", binding.Name, destination, err)
			}
		}()
	}
	wg.Wait()
	return errors.Join(results...)
}

// Close rejects new calls and waits for admitted calls. It does not cancel callers.
// A wait timeout leaves cleanup running; a later Close can wait for the same result.
func (p *Publisher) Close(ctx context.Context) error {
	if ctx == nil {
		return errors.New("event close context is nil")
	}
	p.closeOnce.Do(func() {
		p.StopAccepting()
		go func() { p.active.Wait(); close(p.closeDone) }()
	})
	select {
	case <-p.closeDone:
		return nil
	default:
	}
	select {
	case <-p.closeDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// StopAccepting closes admission before platform shutdown cancels pending creation.
func (p *Publisher) StopAccepting() {
	p.mu.Lock()
	p.closing = true
	p.mu.Unlock()
}
