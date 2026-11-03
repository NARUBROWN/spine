package spine_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	eventadapter "github.com/NARUBROWN/spine/adapter/event"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/event/publish"
)

type publicEventFactory struct {
	consume, publish bool
	reader           func(context.Context, eventadapter.Subscription) (eventadapter.Reader, error)
	publisher        func(context.Context) (eventadapter.Publisher, error)
}

func (f publicEventFactory) Capabilities() eventadapter.Capabilities {
	return eventadapter.Capabilities{ContractVersion: eventadapter.ContractVersion, Consume: f.consume, Publish: f.publish}
}
func (f publicEventFactory) NewReader(ctx context.Context, s eventadapter.Subscription) (eventadapter.Reader, error) {
	return f.reader(ctx, s)
}
func (f publicEventFactory) NewPublisher(ctx context.Context) (eventadapter.Publisher, error) {
	return f.publisher(ctx)
}

type publicEventReader struct {
	deliveries    chan eventadapter.Delivery
	reads, closes atomic.Int32
}

func (r *publicEventReader) Read(ctx context.Context) (eventadapter.Delivery, error) {
	r.reads.Add(1)
	select {
	case d := <-r.deliveries:
		return d, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (r *publicEventReader) Close(context.Context) error { r.closes.Add(1); return nil }

type publicDelivery struct{ ack, nack func(context.Context) error }

func (*publicDelivery) Message() eventadapter.Message {
	return eventadapter.Message{EventName: "producer-controlled-unregistered-route", Payload: []byte(`{"value":"input"}`)}
}
func (d *publicDelivery) Ack(ctx context.Context) error  { return d.ack(ctx) }
func (d *publicDelivery) Nack(ctx context.Context) error { return d.nack(ctx) }

type publicPublisher struct {
	send   func(context.Context, eventadapter.Message) error
	closes atomic.Int32
}

func (p *publicPublisher) Publish(ctx context.Context, m eventadapter.Message) error {
	return p.send(ctx, m)
}
func (p *publicPublisher) Close(context.Context) error { p.closes.Add(1); return nil }

type publicInput struct {
	Value string `json:"value"`
}
type publicEventController struct {
	handle func(context.Context, publicInput) error
}

func (c *publicEventController) Handle(ctx context.Context, input publicInput) error {
	return c.handle(ctx, input)
}

type serializedEvent struct {
	calls *atomic.Int32
	err   error
	at    time.Time
}

func (*serializedEvent) Name() string            { return "result.created" }
func (e *serializedEvent) OccurredAt() time.Time { return e.at }
func (e *serializedEvent) MarshalJSON() ([]byte, error) {
	e.calls.Add(1)
	if e.err != nil {
		return nil, e.err
	}
	return []byte(`{"processed":true}`), nil
}

func TestAppEventExecutesTrustedSubscriptionAndSerializesOnceBeforeAck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var handlers, encoded, sends, acks, nacks, builds atomic.Int32
	at := time.Unix(1_700_000_000, 0)
	reader := &publicEventReader{deliveries: make(chan eventadapter.Delivery, 1)}
	reader.deliveries <- &publicDelivery{ack: func(settlement context.Context) error {
		if sends.Load() != 2 || encoded.Load() != 1 || settlement.Err() != nil {
			t.Error("모든 발행과 한 번의 직렬화를 마친 뒤 확인해야 합니다")
		}
		acks.Add(1)
		cancel()
		return nil
	}, nack: func(context.Context) error { nacks.Add(1); cancel(); return nil }}
	makePublisher := func() *publicPublisher {
		return &publicPublisher{send: func(_ context.Context, m eventadapter.Message) error {
			if m.EventName != "result.created" || string(m.Payload) != `{"processed":true}` || !m.OccurredAt.Equal(at) {
				t.Error("직렬화된 메시지가 다릅니다")
			}
			sends.Add(1)
			return nil
		}}
	}
	first, second := makePublisher(), makePublisher()
	app := spine.New(
		spine.WithEventAdapter("source", publicEventFactory{consume: true, publish: true, reader: func(_ context.Context, s eventadapter.Subscription) (eventadapter.Reader, error) {
			builds.Add(1)
			if s.Topic != "trusted-route" {
				t.Error("구독 키가 다릅니다")
			}
			return reader, nil
		}, publisher: func(context.Context) (eventadapter.Publisher, error) { return first, nil }}),
		spine.WithEventAdapter("fanout", publicEventFactory{publish: true, publisher: func(context.Context) (eventadapter.Publisher, error) { return second, nil }}))
	app.Constructor(func() *publicEventController {
		return &publicEventController{handle: func(ctx context.Context, input publicInput) error {
			if input.Value != "input" {
				t.Error("메시지 DTO가 다릅니다")
			}
			handlers.Add(1)
			publish.Event(ctx, &serializedEvent{calls: &encoded, at: at})
			return nil
		}}
	})
	if err := app.Consumers().Register("trusted-route", (*publicEventController).Handle); err != nil {
		t.Fatal(err)
	}
	if err := app.RunContext(ctx, boot.Options{}); err != nil {
		t.Fatal(err)
	}
	if handlers.Load() != 1 || acks.Load() != 1 || nacks.Load() != 0 || builds.Load() != 1 || reader.closes.Load() != 1 || first.closes.Load() != 1 || second.closes.Load() != 1 {
		t.Fatalf("실행·준비·정리 횟수가 다릅니다: handler=%d ack=%d nack=%d builds=%d", handlers.Load(), acks.Load(), nacks.Load(), builds.Load())
	}
}

func TestAppEventNacksAfterPublisherOrSerializationFailure(t *testing.T) {
	for _, failure := range []string{"publisher", "serialization"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var encoded, sends, acks, nacks atomic.Int32
			cause := errors.New("failure")
			reader := &publicEventReader{deliveries: make(chan eventadapter.Delivery, 1)}
			reader.deliveries <- &publicDelivery{ack: func(context.Context) error { acks.Add(1); cancel(); return nil }, nack: func(context.Context) error { nacks.Add(1); cancel(); return nil }}
			publisher := &publicPublisher{send: func(context.Context, eventadapter.Message) error { sends.Add(1); return cause }}
			app := spine.New(spine.WithEventAdapter("events", publicEventFactory{consume: true, publish: true, reader: func(context.Context, eventadapter.Subscription) (eventadapter.Reader, error) { return reader, nil }, publisher: func(context.Context) (eventadapter.Publisher, error) { return publisher, nil }}))
			app.Constructor(func() *publicEventController {
				return &publicEventController{handle: func(ctx context.Context, _ publicInput) error {
					event := &serializedEvent{calls: &encoded}
					if failure == "serialization" {
						event.err = cause
					}
					publish.Event(ctx, event)
					return nil
				}}
			})
			if err := app.Consumers().Register("trusted-route", (*publicEventController).Handle); err != nil {
				t.Fatal(err)
			}
			if err := app.RunContext(ctx, boot.Options{}); err != nil {
				t.Fatal(err)
			}
			if acks.Load() != 0 || nacks.Load() != 1 || encoded.Load() != 1 || (failure == "serialization" && sends.Load() != 0) {
				t.Fatalf("실패 후 확인이 다릅니다: ack=%d nack=%d encoded=%d sends=%d", acks.Load(), nacks.Load(), encoded.Load(), sends.Load())
			}
		})
	}
}

func TestAppEventStartupCancellationRollsBackPublisher(t *testing.T) {
	publisher := &publicPublisher{}
	started := make(chan struct{})
	app := spine.New(spine.WithEventAdapter("events", publicEventFactory{consume: true, publish: true,
		reader: func(ctx context.Context, _ eventadapter.Subscription) (eventadapter.Reader, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}, publisher: func(context.Context) (eventadapter.Publisher, error) { return publisher, nil }}))
	app.Constructor(func() *publicEventController { return &publicEventController{} })
	if err := app.Consumers().Register("orders", (*publicEventController).Handle); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		prepared, err := app.Prepare(ctx, boot.Options{})
		if prepared != nil {
			t.Error("취소한 준비가 결과를 반환했습니다")
		}
		done <- err
	}()
	select {
	case <-started:
		cancel()
	case <-ctx.Done():
		t.Fatal("reader initialization did not start")
	}
	if err := <-done; !errors.Is(err, context.Canceled) || publisher.closes.Load() != 1 {
		t.Fatalf("시작 취소와 publisher 정리가 다릅니다: %v", err)
	}
}
