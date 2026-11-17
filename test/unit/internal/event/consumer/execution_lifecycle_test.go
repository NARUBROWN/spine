package consumer

import (
	"context"
	"errors"
	"github.com/NARUBROWN/spine/internal/container"
	eventresolver "github.com/NARUBROWN/spine/internal/event/consumer/resolver"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/NARUBROWN/spine/internal/resolver"
	"reflect"
	"testing"
)

func TestConsumerContextPreservesCallerContext(t *testing.T) {
	type key struct{}
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), key{}, "carried"))
	request := execution.NewState(ctx)
	view := execution.NewContext(request, execution.NewInput(execution.Sources{})).ExecutionContext()
	if view.Context() != ctx || view.Context().Value(key{}) != "carried" {
		t.Fatal("context changed")
	}
	cancel()
	if view.Context().Err() != context.Canceled {
		t.Fatal("cancellation lost")
	}
}

// The standard context is canceled before the runtime acknowledges a delivery.
func TestConsumerRuntimeCancelsExecutionBeforeSettlement(t *testing.T) {
	for _, outcome := range []string{"success", "error", "panic"} {
		t.Run(outcome, func(t *testing.T) {
			var captured context.Context
			signals := make(chan string, 1)
			registry := NewRegistry()
			callback := func(ctx context.Context, name string) error {
				if name != "trusted-topic" {
					return errors.New("untrusted event name used")
				}
				captured = ctx
				switch outcome {
				case "error":
					return errors.New("failed")
				case "panic":
					panic("failed")
				}
				return nil
			}
			meta := execution.HandlerMeta{Callback: reflect.ValueOf(callback)}
			registry.registrations = []Registration{{Topic: "trusted-topic", Meta: meta}}
			message := &Message{EventName: "untrusted-name", Payload: []byte("message")}
			settled := func(kind string) func() error {
				return func() error {
					if captured == nil || captured.Err() != context.Canceled {
						signals <- "uncanceled"
					} else {
						signals <- kind
					}
					return nil
				}
			}
			message.SetAckHandler(settled("ack"))
			message.SetNackHandler(settled("nack"))
			run := pipeline.NewPipeline(nil, invoker.NewInvoker(container.New()))
			run.AddArgumentResolver(&resolver.StdContextResolver{}, &eventresolver.EventNameResolver{})
			runtime := NewRuntime(registry, &runtimeTestFactory{reader: &runtimeTestReader{msg: message}}, run)
			runtime.Start(t.Context())
			defer runtime.Stop()
			want := "ack"
			if outcome != "success" {
				want = "nack"
			}
			if got := waitSignal(t, signals); got != want {
				t.Fatalf("settlement=%s want=%s", got, want)
			}
		})
	}
}
