package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	echoplatform "github.com/NARUBROWN/spine-echo-platform"
	kafkaplatform "github.com/NARUBROWN/spine-kafka-platform"
	rabbitmqplatform "github.com/NARUBROWN/spine-rabbitmq-platform"
	eventadapter "github.com/NARUBROWN/spine/contract/event"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/segmentio/kafka-go"
)

type liveEventInput struct {
	ID string `json:"id"`
}
type liveDomainEvent struct {
	ID    string `json:"id"`
	topic string
}

func (e liveDomainEvent) Name() string        { return e.topic }
func (liveDomainEvent) OccurredAt() time.Time { return time.Now() }

type liveEventHTTPController struct{ topic string }

func (c *liveEventHTTPController) Emit(ctx context.Context, body input.Body[liveEventInput]) httpx.Response[string] {
	publish.Event(ctx, liveDomainEvent{ID: body.ID, topic: c.topic})
	return httpx.Response[string]{Body: "accepted"}
}

type liveEventWorker struct {
	topic    string
	attempts *atomic.Int32
}

func (c *liveEventWorker) Handle(ctx context.Context, input liveEventInput) error {
	if c.attempts.Add(1) == 1 {
		return errors.New("fail the first delivery")
	}
	publish.Event(ctx, liveDomainEvent{ID: input.ID, topic: c.topic})
	return nil
}

// readyEventFactory는 공개 계약만으로 초기 구독 준비 완료를 관찰합니다.
type readyEventFactory struct {
	eventadapter.Factory
	ready chan struct{}
	once  sync.Once
}

func (f *readyEventFactory) NewReader(ctx context.Context, subscription eventadapter.Subscription) (eventadapter.Reader, error) {
	reader, err := f.Factory.NewReader(ctx, subscription)
	if err == nil {
		f.once.Do(func() { close(f.ready) })
	}
	return reader, err
}

func TestAppEventLive_HTTPToBrokerWorkerAndResult(t *testing.T) {
	for _, kind := range []string{"kafka", "rabbitmq"} {
		t.Run(kind, func(t *testing.T) {
			broker := os.Getenv("SPINE_TEST_KAFKA_BROKER")
			address := os.Getenv("SPINE_TEST_RABBITMQ_URL")
			if (kind == "kafka" && broker == "") || (kind == "rabbitmq" && address == "") {
				t.Skip("live broker is not configured")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
			defer cancel()
			name := fmt.Sprintf("spine-app-%d", time.Now().UnixNano())
			inputTopic, outputTopic := name+".input", name+".output"
			var makeFactory func(bool, bool, string) (eventadapter.Factory, error)
			if kind == "kafka" {
				conn, err := kafka.DialContext(ctx, "tcp", broker)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				if err := conn.CreateTopics(kafka.TopicConfig{Topic: inputTopic, NumPartitions: 1, ReplicationFactor: 1}, kafka.TopicConfig{Topic: outputTopic, NumPartitions: 1, ReplicationFactor: 1}); err != nil {
					t.Fatal(err)
				}
				defer conn.DeleteTopics(inputTopic, outputTopic)
				makeFactory = func(consume, publish bool, group string) (eventadapter.Factory, error) {
					return kafkaplatform.NewFactory(kafkaplatform.Config{Brokers: []string{broker}, Consume: consume, Publish: publish, GroupID: group, AllowInsecureTransport: true})
				}
			} else {
				conn, err := amqp.Dial(address)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				ch, err := conn.Channel()
				if err != nil {
					t.Fatal(err)
				}
				defer ch.ExchangeDelete(name, false, false)
				defer ch.QueueDelete(inputTopic, false, false, false)
				defer ch.QueueDelete(outputTopic, false, false, false)
				makeFactory = func(consume, publish bool, _ string) (eventadapter.Factory, error) {
					return rabbitmqplatform.NewFactory(rabbitmqplatform.Config{URL: address, Exchange: name, Consume: consume, Publish: publish, AllowInsecureTransport: true, FailurePolicy: rabbitmqplatform.FailureRequeue})
				}
			}
			factory := func(consume, publish bool, group string) eventadapter.Factory {
				f, err := makeFactory(consume, publish, group)
				if err != nil {
					t.Fatal(err)
				}
				return f
			}
			observer, err := factory(true, false, name+".observer").NewReader(ctx, eventadapter.Subscription{Topic: outputTopic})
			if err != nil {
				t.Fatal(err)
			}
			defer observer.Close(context.Background())
			var attempts atomic.Int32
			workerFactory := &readyEventFactory{Factory: factory(true, true, name+".worker"), ready: make(chan struct{})}
			worker := spine.New(spine.WithEventAdapter("events", workerFactory, spine.WithEventConsumerRetry(boot.ConsumerRetryOptions{InitialDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond, MaxAttempts: 3})))
			worker.Constructor(func() *liveEventWorker { return &liveEventWorker{topic: outputTopic, attempts: &attempts} })
			if err := worker.Consumers().Register(inputTopic, (*liveEventWorker).Handle); err != nil {
				t.Fatal(err)
			}
			workerCtx, stopWorker := context.WithCancel(ctx)
			workerDone := make(chan struct{})
			var workerErr error
			started := false
			defer func() {
				stopWorker()
				if !started {
					return
				}
				select {
				case <-workerDone:
					if workerErr != nil {
						t.Error(workerErr)
					}
				case <-time.After(15 * time.Second):
					t.Error("worker shutdown timed out")
				}
			}()

			sender := spine.New(echoplatform.Option(), spine.WithEventAdapter("events", factory(false, true, "")))
			sender.Constructor(func() *liveEventHTTPController { return &liveEventHTTPController{topic: inputTopic} })
			sender.Route(http.MethodPost, "/events", (*liveEventHTTPController).Emit)
			prepared, err := sender.Prepare(ctx, boot.Options{HTTP: &boot.HTTPOptions{}})
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close(context.Background())
			server := httptest.NewServer(mustHTTPHandler(t, prepared))
			defer server.Close()
			sendHTTP := func(id string) int {
				body, err := json.Marshal(liveEventInput{ID: id})
				if err != nil {
					t.Fatal(err)
				}
				request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/events", bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Content-Type", "application/json")
				response, err := http.DefaultClient.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				return response.StatusCode
			}
			if kind == "rabbitmq" {
				// 입력 binding이 없는 첫 요청은 실패합니다. 이후 요청은 새 publisher로 복구해야 합니다.
				if status := sendHTTP("unroutable"); status != http.StatusInternalServerError {
					t.Fatalf("unroutable publish status=%d", status)
				}
			}
			started = true
			go func() { workerErr = worker.RunContext(workerCtx, boot.Options{}); close(workerDone) }()
			select {
			case <-workerFactory.ready:
			case <-workerDone:
				t.Fatalf("worker startup failed: %v", workerErr)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if status := sendHTTP("roundtrip"); status != http.StatusOK {
				t.Fatalf("HTTP publish status=%d", status)
			}

			delivery, err := observer.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var result liveEventInput
			if err := json.Unmarshal(delivery.Message().Payload, &result); err != nil {
				t.Fatal(err)
			}
			if result.ID != "roundtrip" || attempts.Load() < 2 {
				t.Fatalf("App 재전달과 결과가 다릅니다: result=%+v attempts=%d", result, attempts.Load())
			}
			if err := delivery.Ack(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}
