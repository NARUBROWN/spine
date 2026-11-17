package ws

import (
	"context"
	"errors"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
	wsresolver "github.com/NARUBROWN/spine/internal/ws/resolver"
	"github.com/NARUBROWN/spine/pkg/input"
	"github.com/NARUBROWN/spine/pkg/spine"
	publicws "github.com/NARUBROWN/spine/pkg/ws"
	"reflect"
	"testing"
)

type retainedExecutionObserver struct{ retained spine.Ctx }

func (o *retainedExecutionObserver) PreHandle(ctx spine.Ctx, _ core.HandlerMeta) error {
	o.retained = ctx
	ctx.ExecutionContext().Set("owned", true)
	return nil
}
func (*retainedExecutionObserver) PostHandle(spine.Ctx, core.HandlerMeta) {}
func (*retainedExecutionObserver) BeforeResponse(spine.Ctx, core.HandlerMeta, error) error {
	return nil
}
func (*retainedExecutionObserver) AfterCompletion(spine.Ctx, core.HandlerMeta, error) {}

func TestMessageRuntimeReleasesOwnedScopeForEveryOutcome(t *testing.T) {
	for _, outcome := range []string{"success", "error", "panic", "invalid-input"} {
		t.Run(outcome, func(t *testing.T) {
			observer := &retainedExecutionObserver{}
			run := pipeline.NewPipeline(nil, invoker.NewInvoker(container.New()))
			run.AddInterceptor(observer)
			runtime := &Runtime{pipeline: run, argumentResolvers: wsresolver.Resolvers(nil)}
			connectionCtx, cancel := context.WithCancel(t.Context())
			defer cancel()
			callback := func(ctx context.Context, message input.Message[struct{ Text string }]) error {
				switch outcome {
				case "error":
					return errors.New("failed")
				case "panic":
					panic("failed")
				}
				return nil
			}
			payload := []byte(`{"Text":"hello"}`)
			if outcome == "invalid-input" {
				payload = []byte(`{`)
			}
			err := runtime.executeMessage(session{ctx: connectionCtx, id: "connection"}, 1, payload, func(int, []byte) error { return nil }, execution.HandlerMeta{Callback: reflect.ValueOf(callback)})
			if (err != nil) != (outcome != "success") {
				t.Fatal(err)
			}
			if observer.retained.ExecutionContext().Context().Err() != context.Canceled {
				t.Fatal("message context remains active")
			}
			if _, ok := observer.retained.ExecutionContext().Get("owned"); ok {
				t.Fatal("message store retained")
			}
			if err := publicws.Send(observer.retained.ExecutionContext().Context(), 1, nil); !errors.Is(err, publicws.ErrConnectionNotFound) {
				t.Fatal("compatibility sender retained", err)
			}
			if connectionCtx.Err() != nil {
				t.Fatal("message completion canceled connection")
			}
		})
	}
}

var retainedMessageContexts []context.Context

func BenchmarkRuntimeCompletedMessageChurn(b *testing.B) {
	retainedMessageContexts = make([]context.Context, 256)
	next := 0
	callback := func(ctx context.Context, _ input.Message[[]byte]) error {
		retainedMessageContexts[next%len(retainedMessageContexts)] = ctx
		next++
		return nil
	}
	meta := execution.HandlerMeta{Callback: reflect.ValueOf(callback)}
	run := pipeline.NewPipeline(nil, invoker.NewInvoker(container.New()))
	runtime := &Runtime{pipeline: run, argumentResolvers: wsresolver.Resolvers(nil)}
	connection := session{ctx: context.Background(), id: "connection"}
	for n := 0; n < b.N; n++ {
		if err := runtime.executeMessage(connection, 2, make([]byte, 16<<10), nil, meta); err != nil {
			b.Fatal(err)
		}
	}
}
