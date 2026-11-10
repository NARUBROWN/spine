package pipeline_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/event/consumer"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/internal/runtime"
	"github.com/NARUBROWN/spine/internal/transport"
	"github.com/NARUBROWN/spine/internal/ws"
	"github.com/NARUBROWN/spine/pkg/spine"
)

type storeScope struct{ httpcontract.HTTPContext }

func (storeScope) Context() context.Context            { return context.Background() }
func (storeScope) Writer() httpcontract.ResponseWriter { return nil }
func (storeScope) Method() string                      { return "GET" }
func (storeScope) RawPath() string                     { return "/probe" }

type storeObserver struct {
	t         *testing.T
	retained  core.ExecutionContext
	completed bool
}

func (o *storeObserver) PreHandle(ctx core.ExecutionContext, _ core.HandlerMeta) error {
	o.retained = ctx
	ctx.Set("payload", make([]byte, 1024))
	if _, ok := any(ctx).(interface{ ReleaseStore() }); ok {
		o.t.Error("공개 뷰가 저장값의 수명을 변경할 수 있습니다")
	}
	return nil
}
func (*storeObserver) PostHandle(core.ExecutionContext, core.HandlerMeta) {}
func (*storeObserver) BeforeResponse(core.ExecutionContext, core.HandlerMeta, error) error {
	return nil
}
func (o *storeObserver) AfterCompletion(ctx core.ExecutionContext, _ core.HandlerMeta, _ error) {
	if _, ok := ctx.Get("payload"); !ok {
		o.t.Error("완료 훅이 끝나기 전에 저장소가 정리됐습니다")
	}
	ctx.Set("completion", true)
	o.completed = true
}

func TestExecutionReleasesRetainedViewsAfterCompletion(t *testing.T) {
	contexts := map[string]func() execution.Context{
		"http": func() execution.Context {
			return transport.NewHTTPContext(storeScope{}, 0)
		},
		"event": func() execution.Context {
			return consumer.NewRequestContext(context.Background(), &consumer.Message{EventName: "probe"}, nil)
		},
		"websocket": func() execution.Context {
			return ws.NewWSExecutionContext(context.Background(), "conn", "/probe", 1, nil, nil, func(int, []byte) error { return nil })
		},
	}
	for name, create := range contexts {
		t.Run(name, func(t *testing.T) {
			for _, outcome := range []string{"success", "error", "panic"} {
				t.Run(outcome, func(t *testing.T) {
					ctx := create()
					var retainedHandler spine.Ctx = runtime.NewControllerContext(ctx)
					observer := &storeObserver{t: t}
					callback := func() error {
						switch outcome {
						case "error":
							return errors.New("handler failed")
						case "panic":
							panic("handler panic")
						}
						return nil
					}
					routes := router.NewRouter()
					routes.Register(ctx.Method(), ctx.Path(), execution.HandlerMeta{Callback: reflect.ValueOf(callback)})
					run := pipeline.NewPipeline(routes, invoker.NewInvoker(container.New()))
					run.AddInterceptor(observer)
					err := run.Execute(ctx)
					if (err != nil) != (outcome != "success") || !observer.completed {
						t.Fatalf("err=%v completed=%v", err, observer.completed)
					}
					observer.retained.Set("late", make([]byte, 1024))
					for _, key := range []string{"payload", "completion", "late"} {
						if _, ok := observer.retained.Get(key); ok {
							t.Fatalf("보관된 인터셉터 뷰에 값이 남았습니다: %s", key)
						}
						if _, ok := retainedHandler.ExecutionContext().Get(key); ok {
							t.Fatalf("보관된 핸들러 뷰에 값이 남았습니다: %s", key)
						}
					}
				})
			}
		})
	}
}
