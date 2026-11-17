package bootstrap

import (
	"context"
	"errors"
	"github.com/NARUBROWN/spine/core"
	publicPublish "github.com/NARUBROWN/spine/pkg/event/publish"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	eventadapter "github.com/NARUBROWN/spine/contract/event"
	"github.com/NARUBROWN/spine/internal/event/consumer"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type eventFactoryStub struct {
	consume, publish bool
	reader           func(context.Context, eventadapter.Subscription) (eventadapter.Reader, error)
	publisher        func(context.Context) (eventadapter.Publisher, error)
}

func (f eventFactoryStub) Capabilities() eventadapter.Capabilities {
	return eventadapter.Capabilities{ContractVersion: eventadapter.ContractVersion, Consume: f.consume, Publish: f.publish}
}
func (f eventFactoryStub) NewReader(ctx context.Context, sub eventadapter.Subscription) (eventadapter.Reader, error) {
	if f.reader == nil {
		return nil, errors.New("unexpected reader initialization")
	}
	return f.reader(ctx, sub)
}
func (f eventFactoryStub) NewPublisher(ctx context.Context) (eventadapter.Publisher, error) {
	if f.publisher == nil {
		return nil, errors.New("unexpected publisher initialization")
	}
	return f.publisher(ctx)
}

type eventReaderStub struct {
	reads, closes atomic.Int32
	closeErr      error
	started       chan struct{}
}

func (r *eventReaderStub) Read(ctx context.Context) (eventadapter.Delivery, error) {
	r.reads.Add(1)
	if r.started != nil {
		close(r.started)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}
func (r *eventReaderStub) Close(context.Context) error { r.closes.Add(1); return r.closeErr }

type eventPublisherStub struct {
	closes   atomic.Int32
	closeErr error
}

func (*eventPublisherStub) Publish(context.Context, eventadapter.Message) error { return nil }
func (p *eventPublisherStub) Close(context.Context) error                       { p.closes.Add(1); return p.closeErr }

func eventConfig(factory eventadapter.Factory) Config {
	registry := consumer.NewRegistry()
	if err := registry.Register("orders", (*testController).Consume); err != nil {
		panic(err)
	}
	return Config{Constructors: []any{func() *testController { return &testController{} }}, ConsumerRegistry: registry, EventAdapters: []EventAdapterBinding{{Name: "events", Factory: factory, Options: boot.EventOptions{ConsumerRetry: boot.ConsumerRetryOptions{MaxAttempts: 1}}}}}
}

func TestPrepareRetainsReaderWithoutReadingAndCloseReportsErrors(t *testing.T) {
	cause := errors.New("reader close failed")
	r := &eventReaderStub{closeErr: cause}
	p := &eventPublisherStub{}
	var builds atomic.Int32
	factory := eventFactoryStub{consume: true, publish: true, reader: func(context.Context, eventadapter.Subscription) (eventadapter.Reader, error) {
		builds.Add(1)
		return r, nil
	}, publisher: func(context.Context) (eventadapter.Publisher, error) { return p, nil }}
	prepared, err := Prepare(context.Background(), eventConfig(factory))
	if err != nil {
		t.Fatal(err)
	}
	if builds.Load() != 1 || r.reads.Load() != 0 || r.closes.Load() != 0 {
		t.Fatal("Prepare가 reader를 읽거나 재생성했습니다")
	}
	err = prepared.Close(context.Background())
	if !errors.Is(err, cause) {
		t.Fatalf("reader 정리 오류가 사라졌습니다: %v", err)
	}
	if r.closes.Load() != 1 || p.closes.Load() != 1 {
		t.Fatalf("자원 정리 횟수가 다릅니다: reader=%d publisher=%d", r.closes.Load(), p.closes.Load())
	}
	if again := prepared.Close(context.Background()); again != err {
		t.Fatal("종료 결과가 보존되지 않았습니다")
	}
}

func TestRunReusesPreparedReader(t *testing.T) {
	r := &eventReaderStub{started: make(chan struct{})}
	var builds atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	factory := eventFactoryStub{consume: true, reader: func(context.Context, eventadapter.Subscription) (eventadapter.Reader, error) {
		builds.Add(1)
		return r, nil
	}}
	done := make(chan error, 1)
	go func() { done <- RunContext(ctx, eventConfig(factory)) }()
	select {
	case <-r.started:
		cancel()
	case <-ctx.Done():
		t.Fatal("prepared reader did not start")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if builds.Load() != 1 || r.reads.Load() != 1 || r.closes.Load() != 1 {
		t.Fatalf("준비한 reader를 한 번 사용하고 닫아야 합니다: build=%d read=%d close=%d", builds.Load(), r.reads.Load(), r.closes.Load())
	}
}

func TestPrepareRejectsNilEventResourcesAndCleansPartialPublisher(t *testing.T) {
	cause := errors.New("publisher init failed")
	p := &eventPublisherStub{}
	config := Config{EventAdapters: []EventAdapterBinding{{Name: "events", Factory: eventFactoryStub{publish: true, publisher: func(context.Context) (eventadapter.Publisher, error) { return p, cause }}}}}
	if result, err := Prepare(context.Background(), config); result != nil || !errors.Is(err, cause) || p.closes.Load() != 1 {
		t.Fatalf("부분 publisher 자원을 정리하지 않았습니다: %v", err)
	}
	config = eventConfig(eventFactoryStub{consume: true, reader: func(context.Context, eventadapter.Subscription) (eventadapter.Reader, error) {
		return (*eventReaderStub)(nil), nil
	}})
	if result, err := Prepare(context.Background(), config); result != nil || !errors.Is(err, eventadapter.ErrInvalidAdapter) {
		t.Fatalf("typed nil reader를 거부해야 합니다: %v", err)
	}
	config = Config{EventAdapters: []EventAdapterBinding{{Name: "events", Factory: eventFactoryStub{publish: true, publisher: func(context.Context) (eventadapter.Publisher, error) { return (*eventPublisherStub)(nil), nil }}}}}
	if result, err := Prepare(context.Background(), config); result != nil || !errors.Is(err, eventadapter.ErrInvalidAdapter) {
		t.Fatalf("typed nil publisher를 거부해야 합니다: %v", err)
	}
}

func TestEventPreflightHasNoConstructionSideEffects(t *testing.T) {
	factory := eventFactoryStub{consume: true, publish: true}
	if err := Validate(eventConfig(factory)); err != nil {
		t.Fatal(err)
	}
	for _, bindings := range [][]EventAdapterBinding{
		{{Name: "events", Factory: nil}},
		{{Name: "", Factory: factory}},
		{{Name: "same", Factory: factory}, {Name: "same", Factory: factory}},
		{{Name: "events", Factory: factory, Options: boot.EventOptions{SettlementTimeout: -time.Second}}},
		{{Name: "events", Factory: factory, Options: boot.EventOptions{PublisherErrorHistory: boot.CleanupErrorHistoryOptions{MaxEntries: -1}}}},
		{{Name: "events", Factory: factory, Options: boot.EventOptions{ConsumerErrorHistory: boot.CleanupErrorHistoryOptions{MaxAge: -time.Second}}}},
		{{Name: "events", Factory: factory, Options: boot.EventOptions{ConsumerRetry: boot.ConsumerRetryOptions{MaxAttempts: -1}}}},
	} {
		if err := Validate(Config{EventAdapters: bindings}); err == nil {
			t.Fatalf("잘못된 이벤트 구성이 통과했습니다: %+v", bindings)
		}
	}
}

func TestApplicationCloseCancelsPublisherReconnection(t *testing.T) {
	failed := &reconnectPublisher{}
	entered := make(chan struct{})
	var publisher publicPublish.Publisher
	config := Config{
		EventAdapters: []EventAdapterBinding{{Name: "events", Factory: eventFactoryStub{publish: true, publisher: func(ctx context.Context) (eventadapter.Publisher, error) {
			if failed.creates.Add(1) == 1 {
				return failed, nil
			}
			close(entered)
			<-ctx.Done()
			return nil, ctx.Err()
		}}}},
		ShutdownHooks: []any{func(context.Context, publicPublish.Publisher) error { return nil }},
		CustomTransports: []core.CustomTransport{callbackTransport{init: func(c core.Container) error {
			value, err := c.Resolve(reflect.TypeFor[publicPublish.Publisher]())
			if err == nil {
				publisher = value.(publicPublish.Publisher)
			}
			return err
		}}},
	}
	app, err := Prepare(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	if err := publisher.Event(t.Context(), "orders", nil); err == nil {
		t.Fatal("expected initial failure")
	}
	done := make(chan error, 1)
	go func() { done <- publisher.Event(context.Background(), "orders", nil) }()
	<-entered
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := app.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

type reconnectPublisher struct{ creates atomic.Int32 }

func (*reconnectPublisher) Publish(context.Context, eventadapter.Message) error {
	return errors.New("connection lost")
}
func (*reconnectPublisher) Close(context.Context) error { return nil }

func TestPublisherProviderIsReservedWithoutRunningConstructors(t *testing.T) {
	for _, typed := range []bool{false, true} {
		err := Validate(Config{TypedComponents: typed, Constructors: []any{func() publicPublish.Publisher { t.Error("preflight executed a provider"); return nil }}})
		if err == nil || !strings.Contains(err.Error(), "EVENT_PUBLISHER_RESERVED") {
			t.Fatal(err)
		}
	}
}
