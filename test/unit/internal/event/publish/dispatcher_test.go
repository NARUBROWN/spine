package publish

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	eventadapter "github.com/NARUBROWN/spine/contract/event"
	pkgevent "github.com/NARUBROWN/spine/pkg/event/publish"
)

type testDomainEvent struct {
	name string
	at   time.Time
}

func (e testDomainEvent) Name() string          { return e.name }
func (e testDomainEvent) OccurredAt() time.Time { return e.at }

type testPublisher struct {
	called atomic.Int32
	err    error
}

func (p *testPublisher) Publish(ctx context.Context, event eventadapter.Message) error {
	p.called.Add(1)
	return p.err
}

type funcPublisher struct {
	publish func(context.Context, eventadapter.Message) error
}

func (p *funcPublisher) Publish(ctx context.Context, event eventadapter.Message) error {
	return p.publish(ctx, event)
}

func TestDefaultEventDispatcher_ReturnsJoinedErrors(t *testing.T) {
	sentinel := errors.New("publish failed")
	dispatcher, err := NewDefaultEventDispatcher(
		&testPublisher{err: sentinel},
		&testPublisher{},
	)
	if err != nil {
		t.Fatalf("dispatcher 생성 실패: %v", err)
	}

	err = dispatcher.Dispatch(context.Background(), []pkgevent.DomainEvent{
		testDomainEvent{name: "order.created", at: time.Now()},
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("publisher 에러는 join되어 반환되어야 합니다: %v", err)
	}
}

func TestDefaultEventDispatcher_NoErrorReturnsNil(t *testing.T) {
	dispatcher, err := NewDefaultEventDispatcher(&testPublisher{}, &testPublisher{})
	if err != nil {
		t.Fatalf("dispatcher 생성 실패: %v", err)
	}

	if err := dispatcher.Dispatch(context.Background(), []pkgevent.DomainEvent{
		testDomainEvent{name: "order.created", at: time.Now()},
	}); err != nil {
		t.Fatalf("성공 시 nil이어야 합니다: %v", err)
	}
}

func TestNewDefaultEventDispatcher_RejectsNilPublishers(t *testing.T) {
	var typedNil *testPublisher
	for _, publisher := range []EventPublisher{nil, typedNil} {
		if _, err := NewDefaultEventDispatcher(publisher); err == nil {
			t.Fatal("nil publisher는 dispatcher 생성 시 거부되어야 합니다")
		}
	}
}

func TestDefaultEventDispatcher_ProcessesEventsSequentiallyAndPublishersInParallel(t *testing.T) {
	firstEventPublishersStarted := make(chan struct{})
	releaseFirstEvent := make(chan struct{})
	secondEventStarted := make(chan struct{})
	var firstStarts atomic.Int32
	var secondStartOnce sync.Once
	var releaseFirstEventOnce sync.Once
	var firstPublisherCalls atomic.Int32
	var secondPublisherCalls atomic.Int32

	newPublisher := func(calls *atomic.Int32) EventPublisher {
		return &funcPublisher{publish: func(ctx context.Context, event eventadapter.Message) error {
			calls.Add(1)
			switch event.EventName {
			case "first":
				if firstStarts.Add(1) == 2 {
					close(firstEventPublishersStarted)
				}
				<-releaseFirstEvent
			case "second":
				secondStartOnce.Do(func() { close(secondEventStarted) })
			}
			return nil
		}}
	}

	dispatcher, err := NewDefaultEventDispatcher(
		newPublisher(&firstPublisherCalls),
		newPublisher(&secondPublisherCalls),
	)
	if err != nil {
		t.Fatalf("dispatcher 생성 실패: %v", err)
	}
	release := func() {
		releaseFirstEventOnce.Do(func() { close(releaseFirstEvent) })
	}
	defer release()

	done := make(chan error, 1)
	go func() {
		done <- dispatcher.Dispatch(context.Background(), []pkgevent.DomainEvent{
			testDomainEvent{name: "first", at: time.Now()},
			testDomainEvent{name: "second", at: time.Now()},
		})
	}()

	select {
	case <-firstEventPublishersStarted:
	case <-time.After(time.Second):
		t.Fatal("같은 이벤트의 publisher들은 병렬로 시작되어야 합니다")
	}
	select {
	case <-secondEventStarted:
		t.Fatal("다음 이벤트는 이전 이벤트의 모든 publisher가 끝난 뒤 시작되어야 합니다")
	default:
	}

	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("dispatch 실패: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("dispatch가 종료되지 않았습니다")
	}

	if got := firstPublisherCalls.Load(); got != 2 {
		t.Fatalf("첫 번째 publisher는 각 이벤트를 정확히 한 번 발행해야 합니다: %d", got)
	}
	if got := secondPublisherCalls.Load(); got != 2 {
		t.Fatalf("두 번째 publisher는 각 이벤트를 정확히 한 번 발행해야 합니다: %d", got)
	}
}

func TestDefaultEventDispatcher_PassesCancellationAndJoinsPublisherErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sentinel := errors.New("second publisher failed")
	var firstCalls atomic.Int32
	var secondCalls atomic.Int32
	var secondPublisherSawCanceledContext atomic.Bool

	dispatcher, err := NewDefaultEventDispatcher(
		&funcPublisher{publish: func(ctx context.Context, event eventadapter.Message) error {
			firstCalls.Add(1)
			return ctx.Err()
		}},
		&funcPublisher{publish: func(ctx context.Context, event eventadapter.Message) error {
			secondCalls.Add(1)
			secondPublisherSawCanceledContext.Store(ctx.Err() != nil)
			return sentinel
		}},
	)
	if err != nil {
		t.Fatalf("dispatcher 생성 실패: %v", err)
	}

	err = dispatcher.Dispatch(ctx, []pkgevent.DomainEvent{
		testDomainEvent{name: "order.created", at: time.Now()},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("context 취소 오류가 결합되어야 합니다: %v", err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("publisher 오류가 결합되어야 합니다: %v", err)
	}
	if firstCalls.Load() != 1 || secondCalls.Load() != 1 {
		t.Fatalf("취소된 context에서도 각 publisher는 한 번 호출되어야 합니다: first=%d second=%d", firstCalls.Load(), secondCalls.Load())
	}
	if !secondPublisherSawCanceledContext.Load() {
		t.Fatal("취소된 context가 publisher에 전달되어야 합니다")
	}
}

func TestDefaultEventDispatcher_RecoversPublisherPanicsAndReturnsInRegistrationOrder(t *testing.T) {
	firstPanic := errors.New("first panic")
	dispatcher, err := NewDefaultEventDispatcher(
		&funcPublisher{publish: func(context.Context, eventadapter.Message) error {
			panic(firstPanic)
		}},
		&funcPublisher{publish: func(context.Context, eventadapter.Message) error {
			panic("second panic")
		}},
		&testPublisher{},
	)
	if err != nil {
		t.Fatalf("dispatcher 생성 실패: %v", err)
	}

	err = dispatcher.Dispatch(context.Background(), []pkgevent.DomainEvent{
		testDomainEvent{name: "order.created", at: time.Now()},
	})
	if err == nil {
		t.Fatal("publisher panics must become dispatch errors")
	}
	if !errors.Is(err, firstPanic) {
		t.Fatalf("panic error identity must be preserved: %v", err)
	}
	text := err.Error()
	firstIndex := strings.Index(text, "publisher[0]")
	secondIndex := strings.Index(text, "publisher[1]")
	if firstIndex < 0 || secondIndex < 0 || firstIndex >= secondIndex {
		t.Fatalf("panic errors must be joined in publisher registration order: %v", err)
	}
}

func TestDefaultEventDispatcher_ContinuesOtherPublishersAfterPanic(t *testing.T) {
	var healthyCalls atomic.Int32
	dispatcher, err := NewDefaultEventDispatcher(
		&funcPublisher{publish: func(context.Context, eventadapter.Message) error {
			panic("boom")
		}},
		&funcPublisher{publish: func(context.Context, eventadapter.Message) error {
			healthyCalls.Add(1)
			return nil
		}},
	)
	if err != nil {
		t.Fatalf("dispatcher 생성 실패: %v", err)
	}

	if err := dispatcher.Dispatch(context.Background(), []pkgevent.DomainEvent{
		testDomainEvent{name: "order.created", at: time.Now()},
	}); err == nil {
		t.Fatal("recovered panic must be returned")
	}
	if healthyCalls.Load() != 1 {
		t.Fatalf("healthy publishers must still run, calls=%d", healthyCalls.Load())
	}
}
