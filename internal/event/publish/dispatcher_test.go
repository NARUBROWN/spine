package publish

import (
	"context"
	"errors"
	"testing"
	"time"

	pkgevent "github.com/NARUBROWN/spine/pkg/event/publish"
)

type testDomainEvent struct {
	name string
	at   time.Time
}

func (e testDomainEvent) Name() string          { return e.name }
func (e testDomainEvent) OccurredAt() time.Time { return e.at }

type testPublisher struct {
	called int
	err    error
}

func (p *testPublisher) Publish(ctx context.Context, event pkgevent.DomainEvent) error {
	p.called++
	return p.err
}

func TestDefaultEventDispatcher_ReturnsJoinedErrors(t *testing.T) {
	sentinel := errors.New("publish failed")
	dispatcher := &DefaultEventDispatcher{
		Publishers: []EventPublisher{
			&testPublisher{err: sentinel},
			&testPublisher{},
		},
	}

	err := dispatcher.Dispatch(context.Background(), []pkgevent.DomainEvent{
		testDomainEvent{name: "order.created", at: time.Now()},
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("publisher 에러는 join되어 반환되어야 합니다: %v", err)
	}
}

func TestDefaultEventDispatcher_NoErrorReturnsNil(t *testing.T) {
	dispatcher := &DefaultEventDispatcher{
		Publishers: []EventPublisher{
			&testPublisher{},
			&testPublisher{},
		},
	}

	if err := dispatcher.Dispatch(context.Background(), []pkgevent.DomainEvent{
		testDomainEvent{name: "order.created", at: time.Now()},
	}); err != nil {
		t.Fatalf("성공 시 nil이어야 합니다: %v", err)
	}
}
