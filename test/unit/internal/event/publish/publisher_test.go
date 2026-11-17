package publish

import (
	"context"
	"errors"
	event "github.com/NARUBROWN/spine/contract/event"
	public "github.com/NARUBROWN/spine/pkg/event/publish"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type funcPublisher struct {
	publish func(context.Context, event.Message) error
}

func (p *funcPublisher) Publish(ctx context.Context, m event.Message) error { return p.publish(ctx, m) }
func newTestPublisher(t *testing.T, sends ...func(context.Context, event.Message) error) *Publisher {
	t.Helper()
	bindings := make([]Binding, len(sends))
	for i, send := range sends {
		bindings[i] = Binding{Name: []string{"first", "second", "third"}[i], Publisher: &funcPublisher{send}}
	}
	p, err := NewPublisher(bindings...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return p
}

func TestPublisherWaitsForAllPlatformsAndJoinsErrorsInOrder(t *testing.T) {
	first, second := errors.New("one"), errors.New("two")
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	defer close(release)
	p := newTestPublisher(t, func(context.Context, event.Message) error { started <- struct{}{}; <-release; return first }, func(context.Context, event.Message) error { started <- struct{}{}; <-release; return second })
	done := make(chan error, 1)
	go func() { done <- p.Event(t.Context(), "orders", nil) }()
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("platforms did not start in parallel")
		}
	}
	select {
	case <-done:
		t.Fatal("returned before confirmation")
	default:
	}
	// Release without closing twice in the deferred cleanup.
	release <- struct{}{}
	release <- struct{}{}
	err := <-done
	if !errors.Is(err, first) || !errors.Is(err, second) || strings.Index(err.Error(), "first") > strings.Index(err.Error(), "second") {
		t.Fatal(err)
	}
}

func TestPublisherPanicDoesNotPreventOtherPlatform(t *testing.T) {
	cause := errors.New("panic")
	var healthy atomic.Int32
	p := newTestPublisher(t, func(context.Context, event.Message) error { panic(cause) }, func(context.Context, event.Message) error { healthy.Add(1); return nil })
	if err := p.Event(t.Context(), "orders", nil); !errors.Is(err, cause) || healthy.Load() != 1 {
		t.Fatalf("%v calls=%d", err, healthy.Load())
	}
}

type countedJSON struct{ calls *atomic.Int32 }

func (c countedJSON) MarshalJSON() ([]byte, error) { c.calls.Add(1); return []byte(`42`), nil }
func TestPublisherSerializesOnceAndPreservesOutboxMetadata(t *testing.T) {
	var encoded, seen atomic.Int32
	at := time.Unix(123, 0)
	options := []public.Option{public.WithMessageID("outbox-42"), public.WithOccurredAt(at), public.WithMetadata(map[string]string{"trace": "abc"})}
	send := func(_ context.Context, m event.Message) error {
		seen.Add(1)
		if m.EventName != "orders" || string(m.Payload) != `{"id":42}` || !m.OccurredAt.Equal(at) || m.Metadata["message_id"] != "outbox-42" || m.Metadata["trace"] != "abc" {
			t.Errorf("unexpected message: %+v", m)
		}
		return nil
	}
	p := newTestPublisher(t, send, send)
	for range 2 {
		if err := p.Event(t.Context(), "orders", public.Fields{"id": countedJSON{&encoded}}, options...); err != nil {
			t.Fatal(err)
		}
	}
	if encoded.Load() != 2 || seen.Load() != 4 {
		t.Fatalf("encoded=%d sends=%d", encoded.Load(), seen.Load())
	}
}
func TestPublisherValidatesBeforeTransmission(t *testing.T) {
	var sends atomic.Int32
	p := newTestPublisher(t, func(context.Context, event.Message) error { sends.Add(1); return nil })
	if p.Event(nil, "orders", nil) == nil || p.Event(t.Context(), " ", nil) == nil || p.Event(t.Context(), "orders", public.Fields{"bad": make(chan int)}) == nil {
		t.Fatal("invalid call accepted")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := p.Event(canceled, "orders", nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if sends.Load() != 0 {
		t.Fatal("invalid input reached broker")
	}
	unavailable := newTestPublisher(t)
	if err := unavailable.Event(t.Context(), "orders", nil); !errors.Is(err, public.ErrUnavailable) {
		t.Fatal(err)
	}
	var typedNil *funcPublisher
	for _, native := range []EventPublisher{nil, typedNil} {
		if _, err := NewPublisher(Binding{Publisher: native}); err == nil {
			t.Fatal("nil accepted")
		}
	}
}
func TestPublisherCloseRejectsNewCallsAndWaitsForAdmittedCall(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	p := newTestPublisher(t, func(ctx context.Context, _ event.Message) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	callCtx, cancelCall := context.WithCancel(t.Context())
	defer cancelCall()
	done := make(chan error, 1)
	go func() { done <- p.Event(callCtx, "orders", nil) }()
	<-started
	waitCtx, cancelWait := context.WithCancel(t.Context())
	cancelWait()
	if err := p.Close(waitCtx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := p.Event(t.Context(), "orders", nil); !errors.Is(err, event.ErrInvalidState) {
		t.Fatal(err)
	}
	select {
	case <-done:
		t.Fatal("close canceled a caller's operation")
	default:
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := p.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestPlatformRequestCancellationDoesNotCloseSharedPublisher(t *testing.T) {
	started := make(chan struct{})
	var calls atomic.Int32
	native := &contextPublisher{send: func(ctx context.Context, _ event.Message) error {
		if calls.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}}
	p := NewPlatformPublisher(platformFactoryStub{create: func(context.Context) (event.Publisher, error) {
		t.Error("request cancellation recreated shared resource")
		return native, nil
	}}, native)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- p.Publish(ctx, event.Message{}) }()
	<-started
	if err := p.Publish(t.Context(), event.Message{}); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := p.Publish(t.Context(), event.Message{}); err != nil {
		t.Fatal(err)
	}
	if native.closes.Load() != 0 {
		t.Fatal("request cancellation closed shared publisher")
	}
	if err := p.Close(t.Context()); err != nil || native.closes.Load() != 1 {
		t.Fatalf("close=%v count=%d", err, native.closes.Load())
	}
}

type contextPublisher struct {
	send   func(context.Context, event.Message) error
	closes atomic.Int32
}

func (p *contextPublisher) Publish(ctx context.Context, m event.Message) error { return p.send(ctx, m) }
func (p *contextPublisher) Close(context.Context) error                        { p.closes.Add(1); return nil }

func TestPublisherReconnectionReleasesResources(t *testing.T) {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	goroutines := runtime.NumGoroutine()
	var created, closed atomic.Int32
	create := func(context.Context) (event.Publisher, error) {
		created.Add(1)
		return &resourcePublisher{closed: &closed}, nil
	}
	native, _ := create(t.Context())
	managed := NewPlatformPublisher(platformFactoryStub{create: create}, native)
	publisher, err := NewPublisher(Binding{Name: "resource", Publisher: managed})
	if err != nil {
		t.Fatal(err)
	}
	for range 2000 {
		if err := publisher.Event(t.Context(), "orders", public.Fields{"id": 42}); err == nil {
			t.Fatal("expected connection failure")
		}
	}
	publisher.StopAccepting()
	if err := managed.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := publisher.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	managed.mu.Lock()
	retained := len(managed.instances)
	managed.mu.Unlock()
	if retained != 0 || created.Load() != 2000 || closed.Load() != created.Load() {
		t.Fatalf("created=%d closed=%d retained=%d", created.Load(), closed.Load(), retained)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	t.Logf("2000 failed sends: created=%d closed=%d retained=%d goroutines=%d->%d live_heap=%d->%d", created.Load(), closed.Load(), retained, goroutines, runtime.NumGoroutine(), before.HeapAlloc, after.HeapAlloc)
}

type resourcePublisher struct{ closed *atomic.Int32 }

func (*resourcePublisher) Publish(context.Context, event.Message) error {
	return errors.New("connection failed")
}
func (p *resourcePublisher) Close(context.Context) error { p.closed.Add(1); return nil }

func BenchmarkPublisherEvent(b *testing.B) {
	p, err := NewPublisher(Binding{Name: "benchmark", Publisher: &funcPublisher{publish: func(context.Context, event.Message) error { return nil }}})
	if err != nil {
		b.Fatal(err)
	}
	defer p.Close(context.Background())
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := p.Event(context.Background(), "orders", public.Fields{"id": 42}); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestPlatformOperationTimeoutRetiresTransportWithoutCancelingCaller(t *testing.T) {
	var creates atomic.Int32
	first := &contextPublisher{send: func(context.Context, event.Message) error { return context.DeadlineExceeded }}
	next := &contextPublisher{send: func(context.Context, event.Message) error { return nil }}
	p := NewPlatformPublisher(platformFactoryStub{create: func(context.Context) (event.Publisher, error) { creates.Add(1); return next, nil }}, first)
	if err := p.Publish(t.Context(), event.Message{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if err := p.Publish(t.Context(), event.Message{}); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(t.Context()); err != nil || creates.Load() != 1 || first.closes.Load() != 1 || next.closes.Load() != 1 {
		t.Fatalf("close=%v creates=%d first=%d next=%d", err, creates.Load(), first.closes.Load(), next.closes.Load())
	}
}
