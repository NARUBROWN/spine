package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	echo "github.com/NARUBROWN/spine-echo-platform"
	natsplatform "github.com/NARUBROWN/spine-nats-platform"
	event "github.com/NARUBROWN/spine/contract/event"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type order struct {
	ID int `json:"id"`
}

type senderController struct{}

func (*senderController) Send(ctx context.Context, body input.Body[order]) (httpx.Response[string], error) {
	if err := publish.Event(ctx, "order.created", publish.Fields{"id": body.ID}); err != nil {
		return httpx.Response[string]{}, err
	}
	return httpx.Response[string]{Body: "accepted"}, nil
}

type workerController struct{ calls atomic.Int32 }

func (c *workerController) Handle(ctx context.Context, input order) error {
	if c.calls.Add(1) == 1 {
		return errors.New("일시적인 처리 실패")
	}
	return publish.Event(ctx, "order.completed", publish.Fields{"id": input.ID})
}

type observedFactory struct {
	event.Factory
	ready chan struct{}
	acks  chan error
	nacks chan error
}

func (f *observedFactory) NewReader(ctx context.Context, s event.Subscription) (event.Reader, error) {
	r, err := f.Factory.NewReader(ctx, s)
	if err != nil {
		return nil, err
	}
	f.ready <- struct{}{}
	return &observedReader{Reader: r, factory: f}, nil
}

type observedReader struct {
	event.Reader
	factory *observedFactory
}

func (r *observedReader) Read(ctx context.Context) (event.Delivery, error) {
	d, err := r.Reader.Read(ctx)
	if err != nil {
		return nil, err
	}
	return &observedDelivery{Delivery: d, factory: r.factory}, nil
}

type observedDelivery struct {
	event.Delivery
	factory *observedFactory
}

func (d *observedDelivery) Ack(ctx context.Context) error {
	err := d.Delivery.Ack(ctx)
	d.factory.acks <- err
	return err
}

func (d *observedDelivery) Nack(ctx context.Context) error {
	err := d.Delivery.Nack(ctx)
	d.factory.nacks <- err
	return err
}

func TestHTTPPublishWorkerRedeliveryAndResult(t *testing.T) {
	address := os.Getenv("SPINE_TEST_NATS_URL")
	if address == "" {
		t.Fatal("SPINE_TEST_NATS_URL로 실제 JetStream 서버를 지정하세요")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	nc, err := nats.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	stream := fmt.Sprintf("SPINE_APP_%d", time.Now().UnixNano())
	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: stream, Subjects: []string{stream + ".>"}}); err != nil {
		t.Fatal(err)
	}
	defer js.DeleteStream(context.Background(), stream)
	factory := func(consume, publish bool, prefix string) event.Factory {
		f, err := natsplatform.NewFactory(natsplatform.Config{
			URLs: []string{address}, Stream: stream, Consume: consume, Publish: publish, PublishDestinations: testDestinations{"order.created", "order.completed"},
			ConsumerPrefix: prefix, TopicPrefix: stream + ".", AllowInsecureTransport: true,
			FailurePolicy: natsplatform.FailureRequeue, RequeueDelay: 20 * time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	observer, err := factory(true, false, "observer").NewReader(ctx, event.Subscription{Topic: "order.completed"})
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close(context.Background())
	observed := &observedFactory{Factory: factory(true, true, "worker"), ready: make(chan struct{}, 8), acks: make(chan error, 8), nacks: make(chan error, 8)}
	workerCtrl := &workerController{}
	worker := spine.New(spine.WithEventAdapter("nats", observed))
	worker.Constructor(func() *workerController { return workerCtrl })
	if err := worker.Consumers().Register("order.created", (*workerController).Handle); err != nil {
		t.Fatal(err)
	}
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- worker.RunContext(workerCtx, boot.Options{EnableGracefulShutdown: true, ShutdownTimeout: 3 * time.Second})
	}()
	defer func() {
		stop()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("worker 종료 시간 초과")
		}
	}()
	select {
	case <-observed.ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	sender := spine.New(echo.Option(), spine.WithEventAdapter("nats", factory(false, true, "")))
	sender.Constructor(func() *senderController { return &senderController{} })
	sender.Route(http.MethodPost, "/orders", (*senderController).Send)
	prepared, err := sender.Prepare(ctx, boot.Options{HTTP: &boot.HTTPOptions{}})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close(context.Background())
	server := httptest.NewServer(mustHTTPHandler(t, prepared))
	defer server.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/orders", bytes.NewBufferString(`{"id":42}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("HTTP status: %d", response.StatusCode)
	}
	result, err := observer.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var received order
	if err := json.Unmarshal(result.Message().Payload, &received); err != nil || received.ID != 42 {
		t.Fatalf("result: %+v, %v", received, err)
	}
	if err := result.Ack(ctx); err != nil {
		t.Fatal(err)
	}
	for _, settlement := range []<-chan error{observed.nacks, observed.acks} {
		select {
		case err := <-settlement:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if calls := workerCtrl.calls.Load(); calls != 2 {
		t.Fatalf("handler calls: %d", calls)
	}
}

type testDestinations []string

func (d testDestinations) PublishDestinations() []string { return d }
