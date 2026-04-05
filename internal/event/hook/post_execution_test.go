package hook

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/core"
	internalpublish "github.com/NARUBROWN/spine/internal/event/publish"
	pkgevent "github.com/NARUBROWN/spine/pkg/event/publish"
)

type testDomainEvent struct {
	name string
	at   time.Time
}

func (e testDomainEvent) Name() string          { return e.name }
func (e testDomainEvent) OccurredAt() time.Time { return e.at }

type testEventBus struct {
	events     []pkgevent.DomainEvent
	drainCalls int
}

func (b *testEventBus) Publish(events ...pkgevent.DomainEvent) {
	b.events = append(b.events, events...)
}

func (b *testEventBus) Drain() []pkgevent.DomainEvent {
	b.drainCalls++
	ev := b.events
	b.events = nil
	return ev
}

type testDispatcher struct {
	called int
	got    []pkgevent.DomainEvent
	err    error
}

func (d *testDispatcher) Dispatch(ctx context.Context, events []pkgevent.DomainEvent) error {
	d.called++
	d.got = append([]pkgevent.DomainEvent(nil), events...)
	return d.err
}

type testExecutionContext struct {
	bus core.EventBus
}

func (c *testExecutionContext) Context() context.Context     { return context.Background() }
func (c *testExecutionContext) EventBus() core.EventBus      { return c.bus }
func (c *testExecutionContext) Method() string               { return "GET" }
func (c *testExecutionContext) Path() string                 { return "/" }
func (c *testExecutionContext) Params() map[string]string    { return map[string]string{} }
func (c *testExecutionContext) Header(name string) string    { return "" }
func (c *testExecutionContext) PathKeys() []string           { return nil }
func (c *testExecutionContext) Queries() map[string][]string { return map[string][]string{} }
func (c *testExecutionContext) Set(key string, value any)    {}
func (c *testExecutionContext) Get(key string) (any, bool)   { return nil, false }

var _ internalpublish.EventDispatcher = (*testDispatcher)(nil)

func TestEventDispatchHook_IgnoresPriorError(t *testing.T) {
	bus := &testEventBus{}
	bus.Publish(testDomainEvent{name: "order.created", at: time.Now()})

	dispatcher := &testDispatcher{}
	hook := &EventDispatchHook{Dispatcher: dispatcher}

	err := hook.AfterExecution(&testExecutionContext{bus: bus}, nil, errors.New("controller failed"))
	if err != nil {
		t.Fatalf("기존 에러가 있으면 hook은 no-op 이어야 합니다: %v", err)
	}
	if dispatcher.called != 0 {
		t.Fatal("기존 에러가 있으면 dispatch 하면 안 됩니다")
	}
	if bus.drainCalls != 0 {
		t.Fatal("기존 에러가 있으면 bus를 drain 하면 안 됩니다")
	}
}

func TestEventDispatchHook_DispatchesEvents(t *testing.T) {
	bus := &testEventBus{}
	event := testDomainEvent{name: "order.created", at: time.Now()}
	bus.Publish(event)

	dispatcher := &testDispatcher{}
	hook := &EventDispatchHook{Dispatcher: dispatcher}

	if err := hook.AfterExecution(&testExecutionContext{bus: bus}, nil, nil); err != nil {
		t.Fatalf("예상하지 못한 에러입니다: %v", err)
	}
	if dispatcher.called != 1 {
		t.Fatalf("dispatch는 한 번 호출되어야 합니다: %d", dispatcher.called)
	}
	if len(dispatcher.got) != 1 || dispatcher.got[0].Name() != event.Name() {
		t.Fatalf("전달된 이벤트가 잘못되었습니다: %v", dispatcher.got)
	}
}

func TestEventDispatchHook_PropagatesDispatcherError(t *testing.T) {
	bus := &testEventBus{}
	bus.Publish(testDomainEvent{name: "order.created", at: time.Now()})

	dispatcher := &testDispatcher{err: errors.New("publish failed")}
	hook := &EventDispatchHook{Dispatcher: dispatcher}

	err := hook.AfterExecution(&testExecutionContext{bus: bus}, nil, nil)
	if err == nil || err.Error() != "publish failed" {
		t.Fatalf("dispatcher 에러가 전파되어야 합니다: %v", err)
	}
}

func TestEventDispatchHook_NoEventsIsNoop(t *testing.T) {
	dispatcher := &testDispatcher{}
	hook := &EventDispatchHook{Dispatcher: dispatcher}

	if err := hook.AfterExecution(&testExecutionContext{bus: &testEventBus{}}, nil, nil); err != nil {
		t.Fatalf("이벤트가 없으면 no-op 이어야 합니다: %v", err)
	}
	if dispatcher.called != 0 {
		t.Fatal("이벤트가 없으면 dispatch 하면 안 됩니다")
	}
}
