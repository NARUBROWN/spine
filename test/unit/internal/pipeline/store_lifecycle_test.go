package pipeline_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/internal/transport"
	"github.com/NARUBROWN/spine/pkg/spine"
)

type storeScope struct{ httpcontract.HTTPContext }

func (storeScope) Context() context.Context            { return context.Background() }
func (storeScope) Writer() httpcontract.ResponseWriter { return nil }
func (storeScope) Method() string                      { return "GET" }
func (storeScope) RawPath() string                     { return "/probe" }

type storeObserver struct {
	t         *testing.T
	retained  spine.Ctx
	completed bool
}

func (o *storeObserver) PreHandle(ctx spine.Ctx, _ core.HandlerMeta) error {
	o.retained = ctx
	ctx.ExecutionContext().Set("payload", make([]byte, 1024))
	if _, ok := any(ctx).(interface{ ReleaseStore() }); ok {
		o.t.Error("공개 뷰가 저장값의 수명을 변경할 수 있습니다")
	}
	return nil
}
func (*storeObserver) PostHandle(spine.Ctx, core.HandlerMeta) {}
func (*storeObserver) BeforeResponse(spine.Ctx, core.HandlerMeta, error) error {
	return nil
}
func (o *storeObserver) AfterCompletion(ctx spine.Ctx, _ core.HandlerMeta, _ error) {
	if _, ok := ctx.ExecutionContext().Get("payload"); !ok {
		o.t.Error("완료 훅이 끝나기 전에 저장소가 정리됐습니다")
	}
	ctx.ExecutionContext().Set("completion", true)
	o.completed = true
}

func TestExecutionReleasesRetainedViewsAfterCompletion(t *testing.T) {
	contexts := map[string]func() *execution.Input{
		"http": func() *execution.Input {
			request := transport.NewHTTPContext(storeScope{}, 0)
			return execution.NewInput(execution.Sources{HTTPRequest: request, HTTPResponse: request, Routing: request})
		},
		"event": func() *execution.Input {
			return execution.NewInput(execution.Sources{Consumer: &execution.ConsumerMessage{Name: "probe", Payload: make([]byte, 1024)}})
		},
		"websocket": func() *execution.Input {
			return execution.NewInput(execution.Sources{WebSocketRequest: storeHandshake{}, ConnectionID: "conn", WebSocket: &execution.WebSocketMessage{Kind: 1, Payload: make([]byte, 1024)}})
		},
	}

	for name, create := range contexts {
		t.Run(name, func(t *testing.T) {
			for _, outcome := range []string{"success", "error", "panic"} {
				t.Run(outcome, func(t *testing.T) {
					ctx := execution.NewState(context.Background())
					input := create()
					public := execution.NewContext(ctx, input)
					if _, ok := public.HTTPRequestContext(); ok != (name == "http") {
						t.Fatal("전송 종류와 HTTP 뷰 지원 여부가 다릅니다")
					}
					if _, ok := public.WebSocket(); ok != (name == "websocket") {
						t.Fatal("전송 종류와 WebSocket 뷰 지원 여부가 다릅니다")
					}
					views := []any{public, public.ExecutionContext()}
					if request, ok := public.HTTPRequestContext(); ok {
						views = append(views, request)
					}
					if response, ok := public.HTTPResponseContext(); ok {
						views = append(views, response)
					}
					if socket, ok := public.WebSocket(); ok {
						views = append(views, socket)
					}
					for _, view := range views {
						for _, method := range []string{"ReleaseStore", "SetRoute", "EventBus", "SetResponseWriter", "Bind"} {
							if _, exposed := reflect.TypeOf(view).MethodByName(method); exposed {
								t.Fatalf("공개 뷰 %T에 내부 제어 메서드가 노출됐습니다: %s", view, method)
							}
						}
					}
					var retainedHandler spine.Ctx = execution.NewContext(ctx, input)
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
					meta := execution.HandlerMeta{Callback: reflect.ValueOf(callback)}
					run := pipeline.NewPipeline(routes, invoker.NewInvoker(container.New()))
					run.AddInterceptor(observer)
					err := run.ExecuteRegistered(ctx, meta, input)
					if (err != nil) != (outcome != "success") || !observer.completed {
						t.Fatalf("err=%v completed=%v", err, observer.completed)
					}
					if _, ok := public.HTTPRequestContext(); ok {
						t.Fatal("HTTP input retained")
					}
					if _, ok := public.WebSocket(); ok {
						t.Fatal("WS input retained")
					}
					if _, ok := input.ConsumerMessage(); ok {
						t.Fatal("consumer input retained")
					}
					observer.retained.ExecutionContext().Set("late", make([]byte, 1024))
					for _, key := range []string{"payload", "completion", "late"} {
						if _, ok := observer.retained.ExecutionContext().Get(key); ok {
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

type storeHandshake struct {
	execution.WebSocketRequestContext
}

func (storeHandshake) Path() string { return "/probe" }
