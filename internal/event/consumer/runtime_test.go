package consumer

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	eventresolver "github.com/NARUBROWN/spine/internal/event/consumer/resolver"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
)

type runtimeTestRouter struct {
	meta core.HandlerMeta
}

func (r *runtimeTestRouter) Route(ctx core.ExecutionContext) (core.HandlerMeta, error) {
	return r.meta, nil
}

type runtimeTestReader struct {
	msg  *Message
	sent bool
}

func (r *runtimeTestReader) Read(ctx context.Context) (*Message, error) {
	if !r.sent {
		r.sent = true
		return r.msg, nil
	}

	<-ctx.Done()
	return nil, ctx.Err()
}

func (r *runtimeTestReader) Close() error { return nil }

type runtimeTestFactory struct {
	reader Reader
}

func (f *runtimeTestFactory) Build(reg Registration) (Reader, error) {
	return f.reader, nil
}

type runtimeTestPostHook struct {
	err error
}

func (h *runtimeTestPostHook) AfterExecution(ctx core.ExecutionContext, result []any, err error) error {
	return h.err
}

type runtimeTestController struct{}

func (c *runtimeTestController) Handle(payload []byte) {}

func (c *runtimeTestController) Panic(payload []byte) {
	panic("boom")
}

func newRuntimePipeline(t *testing.T, methodName string, hookErr error) *pipeline.Pipeline {
	t.Helper()

	ctr := container.New()
	if err := ctr.RegisterConstructor(func() *runtimeTestController {
		return &runtimeTestController{}
	}); err != nil {
		t.Fatalf("생성자 등록 실패: %v", err)
	}

	controllerType := reflect.TypeOf(&runtimeTestController{})
	method, ok := controllerType.MethodByName(methodName)
	if !ok {
		t.Fatalf("메서드를 찾을 수 없습니다: %s", methodName)
	}

	p := pipeline.NewPipeline(
		&runtimeTestRouter{
			meta: core.HandlerMeta{
				ControllerType: controllerType,
				Method:         method,
			},
		},
		invoker.NewInvoker(ctr),
	)

	p.AddArgumentResolver(&eventresolver.PayloadResolver{})
	if hookErr != nil {
		p.AddPostExecutionHook(&runtimeTestPostHook{err: hookErr})
	}

	return p
}

func waitSignal(t *testing.T, ch <-chan string) string {
	t.Helper()

	select {
	case got := <-ch:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("신호 대기 타임아웃")
		return ""
	}
}

func TestRuntime_NackOnPostHookFailure(t *testing.T) {
	registry := NewRegistry()
	registry.Register("topic", (*runtimeTestController).Handle)

	signals := make(chan string, 2)
	msg := &Message{
		EventName: "topic",
		Payload:   []byte(`hello`),
	}
	msg.SetAckHandler(func() error {
		signals <- "ack"
		return nil
	})
	msg.SetNackHandler(func() error {
		signals <- "nack"
		return nil
	})

	runtime := NewRuntime(
		registry,
		&runtimeTestFactory{reader: &runtimeTestReader{msg: msg}},
		newRuntimePipeline(t, "Handle", context.DeadlineExceeded),
	)

	runtime.Start(context.Background())
	defer runtime.Stop()

	if got := waitSignal(t, signals); got != "nack" {
		t.Fatalf("post hook 실패 시 NACK 되어야 합니다. 실제=%s", got)
	}
}

func TestRuntime_NackOnRecoveredPanic(t *testing.T) {
	registry := NewRegistry()
	registry.Register("topic", (*runtimeTestController).Panic)

	signals := make(chan string, 2)
	msg := &Message{
		EventName: "topic",
		Payload:   []byte(`hello`),
	}
	msg.SetAckHandler(func() error {
		signals <- "ack"
		return nil
	})
	msg.SetNackHandler(func() error {
		signals <- "nack"
		return nil
	})

	runtime := NewRuntime(
		registry,
		&runtimeTestFactory{reader: &runtimeTestReader{msg: msg}},
		newRuntimePipeline(t, "Panic", nil),
	)

	runtime.Start(context.Background())
	defer runtime.Stop()

	if got := waitSignal(t, signals); got != "nack" {
		t.Fatalf("panic 복구 시 NACK 되어야 합니다. 실제=%s", got)
	}
}
