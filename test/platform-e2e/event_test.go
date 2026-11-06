package integration_test

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
	echo "github.com/NARUBROWN/spine-echo-platform"
	kafkaPlatform "github.com/NARUBROWN/spine-kafka-platform"
	rabbitPlatform "github.com/NARUBROWN/spine-rabbitmq-platform"
	event "github.com/NARUBROWN/spine/contract/event"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/NARUBROWN/spine/pkg/httpx"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/segmentio/kafka-go"
)

type validatedInput struct {
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gte=18"`
}
type domainEvent struct {
	validatedInput
	topic string
}

func (e domainEvent) Name() string        { return e.topic }
func (domainEvent) OccurredAt() time.Time { return time.Now() }

type senderController struct {
	topic string
	calls atomic.Int32
}

func (c *senderController) Emit(ctx context.Context, input *validatedInput) httpx.Response[string] {
	c.calls.Add(1)
	publish.Event(ctx, domainEvent{validatedInput: *input, topic: c.topic})
	return httpx.Response[string]{Body: "accepted"}
}

type workerController struct {
	topic string
	calls atomic.Int32
}

func (c *workerController) Handle(ctx context.Context, input validatedInput) {
	c.calls.Add(1)
	publish.Event(ctx, domainEvent{validatedInput: input, topic: c.topic})
}

// 실제 브로커와 공개 계약 사이에서 readiness·결과 확인만 관찰합니다.
type observedFactory struct {
	event.Factory
	ready       chan struct{}
	once        sync.Once
	acks, nacks chan error
}

func (f *observedFactory) NewReader(ctx context.Context, s event.Subscription) (event.Reader, error) {
	r, err := f.Factory.NewReader(ctx, s)
	if err != nil {
		return nil, err
	}
	f.once.Do(func() { close(f.ready) })
	return &observedReader{Reader: r, f: f}, nil
}

type observedReader struct {
	event.Reader
	f *observedFactory
}

func (r *observedReader) Read(ctx context.Context) (event.Delivery, error) {
	d, err := r.Reader.Read(ctx)
	if err != nil {
		return nil, err
	}
	return &observedDelivery{Delivery: d, f: r.f}, nil
}

type observedDelivery struct {
	event.Delivery
	f *observedFactory
}

func (d *observedDelivery) Ack(ctx context.Context) error {
	err := d.Delivery.Ack(ctx)
	select {
	case d.f.acks <- err:
	default:
	}
	return err
}
func (d *observedDelivery) Nack(ctx context.Context) error {
	err := d.Delivery.Nack(ctx)
	select {
	case d.f.nacks <- err:
	default:
	}
	return err
}

type observedValidator struct {
	validation.Validator
	failures chan error
}

func (v observedValidator) Validate(ctx context.Context, input any) error {
	err := v.Validator.Validate(ctx, input)
	if err != nil {
		select {
		case v.failures <- err:
		default:
		}
	}
	return err
}

func TestFinalE2EHTTPValidatorBrokerConsumer(t *testing.T) {
	for _, kind := range []string{"kafka", "rabbitmq"} {
		t.Run(kind, func(t *testing.T) {
			broker, url := os.Getenv("SPINE_TEST_KAFKA_BROKER"), os.Getenv("SPINE_TEST_RABBITMQ_URL")
			if (kind == "kafka" && broker == "") || (kind == "rabbitmq" && url == "") {
				t.Skip("실제 브로커 설정이 필요합니다")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
			defer cancel()
			name := fmt.Sprintf("spine-final-%d", time.Now().UnixNano())
			input, output := name+".input", name+".output"
			var factory func(bool, bool, string) event.Factory
			if kind == "kafka" {
				conn, err := kafka.DialContext(ctx, "tcp", broker)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				if err := conn.CreateTopics(kafka.TopicConfig{Topic: input, NumPartitions: 1, ReplicationFactor: 1}, kafka.TopicConfig{Topic: output, NumPartitions: 1, ReplicationFactor: 1}); err != nil {
					t.Fatal(err)
				}
				defer conn.DeleteTopics(input, output)
				factory = func(consume, publish bool, group string) event.Factory {
					f, err := kafkaPlatform.NewFactory(kafkaPlatform.Config{Brokers: []string{broker}, Consume: consume, Publish: publish, GroupID: group, AllowInsecureTransport: true})
					if err != nil {
						t.Fatal(err)
					}
					return f
				}
			} else {
				conn, err := amqp.Dial(url)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				ch, err := conn.Channel()
				if err != nil {
					t.Fatal(err)
				}
				defer ch.Close()
				defer ch.ExchangeDelete(name, false, false)
				defer ch.QueueDelete(input, false, false, false)
				defer ch.QueueDelete(output, false, false, false)
				factory = func(consume, publish bool, _ string) event.Factory {
					f, err := rabbitPlatform.NewFactory(rabbitPlatform.Config{URL: url, Exchange: name, Consume: consume, Publish: publish, AllowInsecureTransport: true, FailurePolicy: rabbitPlatform.FailureReject})
					if err != nil {
						t.Fatal(err)
					}
					return f
				}
			}
			observer, err := factory(true, false, name+".observer").NewReader(ctx, event.Subscription{Topic: output})
			if err != nil {
				t.Fatal(err)
			}
			defer observer.Close(context.Background())
			observed := &observedFactory{Factory: factory(true, true, name+".worker"), ready: make(chan struct{}), acks: make(chan error, 8), nacks: make(chan error, 8)}
			validator := observedValidator{Validator: newRealValidator(t), failures: make(chan error, 8)}
			workerCtrl := &workerController{topic: output}
			worker := spine.New(spine.WithValidator(validator), spine.WithEventAdapter("events", observed, spine.WithEventConsumerRetry(boot.ConsumerRetryOptions{InitialDelay: 10 * time.Millisecond, MaxDelay: 100 * time.Millisecond, MaxAttempts: 5})))
			worker.Constructor(func() *workerController { return workerCtrl })
			if err := worker.Consumers().Register(input, (*workerController).Handle); err != nil {
				t.Fatal(err)
			}
			workerCtx, stopWorker := context.WithCancel(ctx)
			done := make(chan error, 1)
			go func() {
				done <- worker.RunContext(workerCtx, boot.Options{EnableGracefulShutdown: true, ShutdownTimeout: 10 * time.Second})
			}()
			defer func() {
				stopWorker()
				select {
				case err := <-done:
					if err != nil {
						t.Error(err)
					}
				case <-time.After(15 * time.Second):
					t.Error("컨슈머 종료 시간 초과")
				}
			}()
			select {
			case <-observed.ready:
			case err := <-done:
				t.Fatalf("컨슈머 준비 실패: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			senderCtrl := &senderController{topic: input}
			sender := spine.New(echo.Option(), spine.WithValidator(newRealValidator(t)), spine.WithEventAdapter("events", factory(false, true, "")))
			sender.Constructor(func() *senderController { return senderCtrl })
			sender.Route(http.MethodPost, "/events", (*senderController).Emit)
			prepared, err := sender.Prepare(ctx, boot.Options{HTTP: &boot.HTTPOptions{}})
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close(context.Background())
			srv := httptest.NewServer(mustHTTPHandler(t, prepared))
			defer srv.Close()
			send := func(body string) int {
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/events", bytes.NewBufferString(body))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Content-Type", "application/json")
				resp, err := srv.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				return resp.StatusCode
			}
			if status := send(`{}`); status != 400 || senderCtrl.calls.Load() != 0 {
				t.Fatalf("HTTP 규칙 위반: %d calls=%d", status, senderCtrl.calls.Load())
			}
			if status := send(`{"email":"kim@example.com","age":20}`); status != 200 {
				t.Fatalf("HTTP 정상 입력: %d", status)
			}
			result, err := observer.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var received validatedInput
			if err := json.Unmarshal(result.Message().Payload, &received); err != nil {
				t.Fatal(err)
			}
			if received.Email != "kim@example.com" || received.Age != 20 {
				t.Fatalf("결과 이벤트: %+v", received)
			}
			if err := result.Ack(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-observed.acks:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			publisher, err := factory(false, true, "").NewPublisher(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer publisher.Close(context.Background())
			// HTTP 검증을 우회한 외부 생산자의 잘못된 메시지도 컨슈머가 거부해야 합니다.
			if err := publisher.Publish(ctx, event.Message{EventName: input, Payload: []byte(`{"email":"private-invalid","age":0}`), OccurredAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-validator.failures:
				failure, ok := validation.AsError(err)
				if !ok || len(failure.Fields) != 2 {
					t.Fatalf("실제 검증기 결과: %v", err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			select {
			case err := <-observed.nacks:
				if kind == "rabbitmq" && err != nil {
					t.Fatal(err)
				}
				if kind == "kafka" && !errors.Is(err, event.ErrReaderInvalidated) {
					t.Fatalf("실패한 Kafka offset은 reader를 무효화해야 합니다: %v", err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			stopWorker()
			if workerCtrl.calls.Load() != 1 || senderCtrl.calls.Load() != 1 {
				t.Fatalf("잘못된 입력의 핸들러가 실행됐습니다: worker=%d sender=%d", workerCtrl.calls.Load(), senderCtrl.calls.Load())
			}
		})
	}
}
