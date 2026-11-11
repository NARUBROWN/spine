package bootstrap

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/event/consumer"
)

type consumerValidationDTO struct {
	Name string `json:"name"`
}
type consumerValidationController struct{ calls atomic.Int32 }

func (c *consumerValidationController) Handle(consumerValidationDTO) { c.calls.Add(1) }

type consumerValidator func(context.Context, any) error

func (f consumerValidator) Validate(ctx context.Context, value any) error { return f(ctx, value) }

type validationReader struct {
	message *consumer.Message
	sent    bool
}

func (r *validationReader) Read(ctx context.Context) (*consumer.Message, error) {
	if !r.sent {
		r.sent = true
		return r.message, nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}
func (*validationReader) Close() error { return nil }

type validationReaderFactory struct{ reader consumer.Reader }

func (f validationReaderFactory) Build(consumer.Registration) (consumer.Reader, error) {
	return f.reader, nil
}

func TestConsumerValidationBlocksHandlerAndNacksDelivery(t *testing.T) {
	for _, reject := range []bool{false, true} {
		registry := consumer.NewRegistry()
		if err := registry.Register("users", (*consumerValidationController).Handle); err != nil {
			t.Fatal(err)
		}
		controller := &consumerValidationController{}
		ctr := container.New()
		if err := ctr.RegisterConstructor(func() *consumerValidationController { return controller }); err != nil {
			t.Fatal(err)
		}
		var validated atomic.Int32
		validator := consumerValidator(func(_ context.Context, value any) error {
			validated.Add(1)
			dto, ok := value.(consumerValidationDTO)
			if !ok || dto.Name != "decoded" {
				return errors.New("unexpected DTO")
			}
			if reject {
				return validation.NewError(validation.FieldError{Field: "/name", Rule: "custom"})
			}
			return nil
		})
		p := buildConsumerPipeline(ctr, nil, validator)
		ctx, cancel := context.WithCancel(context.Background())
		message := &consumer.Message{EventName: "users", Payload: []byte(`{"name":"decoded"}`)}
		signals := make(chan string, 1)
		message.SetAckHandler(func() error { signals <- "ack"; return nil })
		message.SetNackHandler(func() error { signals <- "nack"; return nil })
		runtime := consumer.NewRuntime(registry, validationReaderFactory{reader: &validationReader{message: message}}, p)
		runtime.Start(ctx)
		select {
		case got := <-signals:
			want, wantCalls := "ack", int32(1)
			if reject {
				want, wantCalls = "nack", 0
			}
			if got != want || controller.calls.Load() != wantCalls || validated.Load() != 1 {
				t.Errorf("결과=%s 핸들러=%d 검증=%d", got, controller.calls.Load(), validated.Load())
			}
		case <-time.After(3 * time.Second):
			t.Error("검증 결과가 전달 확인으로 이어지지 않았습니다")
		}
		cancel()
		runtime.Stop()
	}
}
