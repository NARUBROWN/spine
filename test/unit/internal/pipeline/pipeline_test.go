package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/event/emission"
	"github.com/NARUBROWN/spine/internal/event/hook"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/handler"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/path"
)

type testEventBus struct{}

func (b *testEventBus) Publish(events ...publish.DomainEvent) {}
func (b *testEventBus) Drain() []publish.DomainEvent          { return nil }

type testExecutionContext struct {
	writer   core.ResponseWriter
	method   string
	path     string
	params   map[string]string
	pathKeys []string
	queries  map[string][]string
	headers  map[string]string
	store    map[string]any
}

func newTestExecutionContext() *testExecutionContext {
	return &testExecutionContext{
		method:  "GET",
		path:    "/",
		params:  map[string]string{},
		queries: map[string][]string{},
		headers: map[string]string{},
		store:   map[string]any{},
	}
}

func (c *testExecutionContext) Context() context.Context     { return context.Background() }
func (c *testExecutionContext) EventBus() emission.Bus       { return &testEventBus{} }
func (c *testExecutionContext) Method() string               { return c.method }
func (c *testExecutionContext) Path() string                 { return c.path }
func (c *testExecutionContext) Params() map[string]string    { return c.params }
func (c *testExecutionContext) Header(name string) string    { return c.headers[name] }
func (c *testExecutionContext) PathKeys() []string           { return c.pathKeys }
func (c *testExecutionContext) Queries() map[string][]string { return c.queries }
func (c *testExecutionContext) Set(key string, value any)    { c.store[key] = value }
func (c *testExecutionContext) Get(key string) (any, bool)   { v, ok := c.store[key]; return v, ok }

type testRouter struct {
	meta execution.HandlerMeta
	err  error
}

func (r *testRouter) Route(ctx execution.Context) (execution.HandlerMeta, error) {
	if r.err != nil {
		return execution.HandlerMeta{}, r.err
	}
	return r.meta, nil
}

type countingRouter struct {
	called int
	err    error
}

func (r *countingRouter) Route(ctx execution.Context) (execution.HandlerMeta, error) {
	r.called++
	if r.err != nil {
		return execution.HandlerMeta{}, r.err
	}
	return execution.HandlerMeta{}, nil
}

type testInterceptor struct {
	name      string
	events    *[]string
	preErr    error
	beforeErr error
	before    func(core.HandlerMeta, error)
	after     func(core.HandlerMeta, error)
}

func (i *testInterceptor) PreHandle(ctx core.ExecutionContext, meta core.HandlerMeta) error {
	*i.events = append(*i.events, "pre:"+i.name)
	return i.preErr
}
func (i *testInterceptor) PostHandle(ctx core.ExecutionContext, meta core.HandlerMeta) {
	*i.events = append(*i.events, "post:"+i.name)
}
func (i *testInterceptor) BeforeResponse(ctx core.ExecutionContext, meta core.HandlerMeta, err error) error {
	*i.events = append(*i.events, "before:"+i.name)
	if i.before != nil {
		i.before(meta, err)
	}
	return i.beforeErr
}
func (i *testInterceptor) AfterCompletion(ctx core.ExecutionContext, meta core.HandlerMeta, err error) {
	*i.events = append(*i.events, "after:"+i.name)
	if i.after != nil {
		i.after(meta, err)
	}
}

type testArgumentResolver struct {
	supports func(pm resolver.ParameterMeta) bool
	resolve  func(ctx execution.Context, pm resolver.ParameterMeta) (any, error)
}

func (r *testArgumentResolver) Supports(pm resolver.ParameterMeta) bool {
	return r.supports(pm)
}
func (r *testArgumentResolver) Resolve(ctx execution.Context, pm resolver.ParameterMeta) (any, error) {
	return r.resolve(ctx, pm)
}

type testReturnHandler struct {
	supports func(rt reflect.Type) bool
	handle   func(v any, ctx execution.Context) error
}

func (h *testReturnHandler) Supports(rt reflect.Type) bool {
	return h.supports(rt)
}
func (h *testReturnHandler) Handle(v any, ctx execution.Context) error {
	return h.handle(v, ctx)
}

type testPostHook struct {
	called      bool
	returnedErr error
}

func (h *testPostHook) AfterExecution(ctx execution.Context) error {
	h.called = true
	return h.returnedErr
}

func TestExecute_ControllerFailureSkipsPostExecutionHook(t *testing.T) {
	called := 0
	p, _ := newPipelineWithController(t, "Fail", &called)
	postHook := &testPostHook{}
	p.AddPostExecutionHook(postHook)
	err := p.Execute(newTestExecutionContext())
	if err == nil || !strings.Contains(err.Error(), "boom") || called != 1 {
		t.Fatalf("컨트롤러 오류를 보존해야 합니다: called=%d err=%v", called, err)
	}
	if postHook.called {
		t.Fatal("컨트롤러가 실패하면 성공 후처리 훅을 호출하면 안 됩니다")
	}
}

func TestStagedJSONValidatesBeforeCommitAndCopiesPayload(t *testing.T) {
	target := &testResponseWriter{}
	stage := newStagedResponseWriter(target)
	if err := stage.WriteJSON(200, json.RawMessage(`{"invalid":`)); err == nil || stage.IsCommitted() || target.writes != 0 {
		t.Fatalf("잘못된 JSON은 응답 준비 전에 거부해야 합니다: err=%v writes=%d", err, target.writes)
	}
	payload := json.RawMessage(`{"value":"before"}`)
	if err := stage.WriteJSON(201, payload); err != nil {
		t.Fatal(err)
	}
	payload[10] = 'X'
	if target.writes != 0 {
		t.Fatal("준비한 응답을 flush 전에 전송하면 안 됩니다")
	}
	if err := stage.flush(); err != nil {
		t.Fatal(err)
	}
	if got := string(target.body.(json.RawMessage)); got != `{"value":"before"}` || target.status != 201 {
		t.Fatalf("준비 뒤 원본 바이트 변경이 전송 응답을 바꾸면 안 됩니다: %q", got)
	}
}

type testResponseWriter struct {
	committed bool
	status    int
	body      any
	writes    int
	writeErr  error
}

func (w *testResponseWriter) SetHeader(key, value string) {}
func (w *testResponseWriter) AddHeader(key, value string) {}
func (w *testResponseWriter) IsCommitted() bool           { return w.committed }
func (w *testResponseWriter) WriteStatus(status int) error {
	w.committed = true
	w.status = status
	w.writes++
	return w.writeErr
}
func (w *testResponseWriter) WriteJSON(status int, value json.RawMessage) error {
	w.committed = true
	w.status = status
	w.body = value
	w.writes++
	return w.writeErr
}
func (w *testResponseWriter) WriteString(status int, value string) error {
	w.committed = true
	w.status = status
	w.body = value
	w.writes++
	return w.writeErr
}
func (w *testResponseWriter) WriteBytes(status int, value []byte) error {
	w.committed = true
	w.status = status
	w.body = value
	w.writes++
	return w.writeErr
}

type testController struct {
	called *int
}

func (c *testController) Handle(v int) string {
	*c.called = *c.called + 1
	return "ok"
}

func (c *testController) Fail() (string, error) {
	*c.called = *c.called + 1
	return "ignored", errors.New("boom")
}

func (c *testController) Panic() string {
	*c.called = *c.called + 1
	panic("boom")
}

type pathController struct{}

func (c *pathController) Mixed(id path.Int, count int, name path.String) {}

type invalidJSONController struct{}

func (*invalidJSONController) Handle() httpx.Response[any] {
	return httpx.Response[any]{Body: make(chan int)}
}

type invalidCookieController struct{}

func (*invalidCookieController) Handle() httpx.Response[string] {
	return httpx.Response[string]{
		Body: "ok",
		Options: httpx.ResponseOptions{
			Cookies: []httpx.Cookie{{Name: "session\r\nInjected", Value: "value"}},
		},
	}
}

func newPipelineWithController(t *testing.T, methodName string, called *int) (*Pipeline, execution.HandlerMeta) {
	t.Helper()

	ctr := container.New()
	if err := ctr.RegisterConstructor(func() *testController {
		return &testController{called: called}
	}); err != nil {
		t.Fatalf("생성자 등록에 실패했습니다: %v", err)
	}

	controllerType := reflect.TypeOf(&testController{})
	method, ok := controllerType.MethodByName(methodName)
	if !ok {
		t.Fatalf("메서드를 찾을 수 없습니다: %s", methodName)
	}

	meta := execution.HandlerMeta{
		ControllerType: controllerType,
		Method:         method,
	}

	p := NewPipeline(&testRouter{meta: meta}, invoker.NewInvoker(ctr))
	return p, meta
}

func TestExecute_SuccessFlow(t *testing.T) {
	controllerCalled := 0
	p, meta := newPipelineWithController(t, "Handle", &controllerCalled)

	events := []string{}
	globalInterceptor := &testInterceptor{name: "global", events: &events}
	routeInterceptor := &testInterceptor{name: "route", events: &events}
	meta.Interceptors = []core.Interceptor{routeInterceptor}
	p.router = &testRouter{meta: meta}
	p.AddInterceptor(globalInterceptor)

	p.AddArgumentResolver(&testArgumentResolver{
		supports: func(pm resolver.ParameterMeta) bool { return pm.Type.Kind() == reflect.Int },
		resolve:  func(ctx execution.Context, pm resolver.ParameterMeta) (any, error) { return 7, nil },
	})

	handled := false
	p.AddReturnValueHandler(&testReturnHandler{
		supports: func(rt reflect.Type) bool { return rt.Kind() == reflect.String },
		handle: func(v any, ctx execution.Context) error {
			events = append(events, "handle:success")
			handled = true
			if v.(string) != "ok" {
				t.Fatalf("예상하지 못한 반환값입니다: %v", v)
			}
			return nil
		},
	})

	postHook := &testPostHook{}
	p.AddPostExecutionHook(postHook)

	err := p.Execute(newTestExecutionContext())
	if err != nil {
		t.Fatalf("실행에 실패했습니다: %v", err)
	}
	if controllerCalled != 1 {
		t.Fatalf("컨트롤러는 한 번 호출되어야 합니다. 실제 호출 횟수: %d", controllerCalled)
	}
	if !handled {
		t.Fatal("리턴 핸들러가 호출되지 않았습니다")
	}
	if !postHook.called {
		t.Fatal("실행 후 훅이 호출되지 않았습니다")
	}

	expected := []string{
		"pre:global",
		"pre:route",
		"handle:success",
		"post:route",
		"post:global",
		"before:route",
		"before:global",
		"after:route",
		"after:global",
	}
	if len(events) != len(expected) {
		t.Fatalf("예상하지 못한 인터셉터 이벤트 개수입니다: %v", events)
	}
	for i := range expected {
		if events[i] != expected[i] {
			t.Fatalf("이벤트 순서가 예상과 다릅니다 (인덱스 %d): 실제 %s, 기대 %s", i, events[i], expected[i])
		}
	}
}

func TestExecute_PostHookFailureDiscardsPreparedSuccessResponse(t *testing.T) {
	controllerCalled := 0
	p, _ := newPipelineWithController(t, "Handle", &controllerCalled)

	p.AddArgumentResolver(&testArgumentResolver{
		supports: func(pm resolver.ParameterMeta) bool { return pm.Type.Kind() == reflect.Int },
		resolve:  func(ctx execution.Context, pm resolver.ParameterMeta) (any, error) { return 7, nil },
	})

	handled := false
	p.AddReturnValueHandler(&testReturnHandler{
		supports: func(rt reflect.Type) bool { return rt.Kind() == reflect.String },
		handle: func(v any, ctx execution.Context) error {
			handled = true
			rw := ctx.(core.HTTPResponseContext).ResponseWriter()
			return rw.WriteStatus(204)
		},
	})

	postHook := &testPostHook{returnedErr: errors.New("dispatch failed")}
	p.AddPostExecutionHook(postHook)

	ctx := newTestExecutionContext()
	writer := &testResponseWriter{}
	ctx.SetResponseWriter(writer)

	err := p.Execute(ctx)
	if err == nil {
		t.Fatal("post hook 실패는 실행 오류로 전파되어야 합니다")
	}
	if !postHook.called {
		t.Fatal("post hook이 호출되어야 합니다")
	}
	if !handled {
		t.Fatal("post hook 전에 성공 응답을 staging해야 합니다")
	}
	if writer.status != 500 || writer.writes != 1 {
		t.Fatalf("post hook 실패는 staged 성공 응답 대신 500 하나만 기록해야 합니다: status=%d writes=%d", writer.status, writer.writes)
	}
}

func TestExecute_RecoversFromPanic(t *testing.T) {
	controllerCalled := 0
	p, _ := newPipelineWithController(t, "Panic", &controllerCalled)

	ctx := newTestExecutionContext()
	writer := &testResponseWriter{}
	ctx.SetResponseWriter(writer)

	err := p.Execute(ctx)
	if err == nil {
		t.Fatal("panic은 error로 복구되어야 합니다")
	}
	if controllerCalled != 1 {
		t.Fatalf("컨트롤러는 한 번 호출되어야 합니다. 실제 호출 횟수: %d", controllerCalled)
	}
	if writer.status != 500 {
		t.Fatalf("panic 복구 시 500 응답이 작성되어야 합니다: %d", writer.status)
	}
}

func TestExecute_AbortByInterceptor(t *testing.T) {
	controllerCalled := 0
	p, meta := newPipelineWithController(t, "Handle", &controllerCalled)

	events := []string{}
	globalInterceptor := &testInterceptor{name: "global", events: &events}
	routeInterceptor := &testInterceptor{name: "route", events: &events, preErr: core.ErrAbortPipeline}
	meta.Interceptors = []core.Interceptor{routeInterceptor}
	p.router = &testRouter{meta: meta}
	p.AddInterceptor(globalInterceptor)

	resolverCalled := false
	p.AddArgumentResolver(&testArgumentResolver{
		supports: func(pm resolver.ParameterMeta) bool { return pm.Type.Kind() == reflect.Int },
		resolve: func(ctx execution.Context, pm resolver.ParameterMeta) (any, error) {
			resolverCalled = true
			return 1, nil
		},
	})

	err := p.Execute(newTestExecutionContext())
	if err != nil {
		t.Fatalf("실행은 에러 없이 중단되어야 합니다. 실제 에러: %v", err)
	}
	if controllerCalled != 0 {
		t.Fatalf("중단 시 컨트롤러가 호출되면 안 됩니다. 실제 호출 횟수: %d", controllerCalled)
	}
	if resolverCalled {
		t.Fatal("라우트 인터셉터가 중단한 요청의 인자를 해석하면 안 됩니다")
	}

	expected := []string{"pre:global", "pre:route", "before:global", "after:route", "after:global"}
	if len(events) != len(expected) {
		t.Fatalf("예상하지 못한 인터셉터 이벤트 개수입니다: %v", events)
	}
	for i := range expected {
		if events[i] != expected[i] {
			t.Fatalf("이벤트 순서가 예상과 다릅니다 (인덱스 %d): 실제 %s, 기대 %s", i, events[i], expected[i])
		}
	}
}

func TestExecute_GlobalInterceptorCanAbortBeforeRouting(t *testing.T) {
	route := &countingRouter{err: errors.New("route should not be called")}
	p := NewPipeline(route, invoker.NewInvoker(container.New()))

	events := []string{}
	globalInterceptor := &testInterceptor{name: "global", events: &events, preErr: core.ErrAbortPipeline}
	p.AddInterceptor(globalInterceptor)

	err := p.Execute(newTestExecutionContext())
	if err != nil {
		t.Fatalf("전역 인터셉터 중단은 에러 없이 종료되어야 합니다. 실제 에러: %v", err)
	}
	if route.called != 0 {
		t.Fatalf("전역 인터셉터가 중단하면 라우터는 호출되면 안 됩니다. 실제 호출 횟수: %d", route.called)
	}

	expected := []string{"pre:global", "after:global"}
	if len(events) != len(expected) {
		t.Fatalf("예상하지 못한 인터셉터 이벤트 개수입니다: %v", events)
	}
	for i := range expected {
		if events[i] != expected[i] {
			t.Fatalf("이벤트 순서가 예상과 다릅니다 (인덱스 %d): 실제 %s, 기대 %s", i, events[i], expected[i])
		}
	}
}

func TestExecute_GlobalAfterCompletionReceivesResolvedMeta(t *testing.T) {
	controllerCalled := 0
	p, meta := newPipelineWithController(t, "Handle", &controllerCalled)

	var got core.HandlerMeta
	events := []string{}
	p.AddInterceptor(&testInterceptor{
		name:   "global",
		events: &events,
		after: func(received core.HandlerMeta, err error) {
			got = received
		},
	})
	p.router = &testRouter{meta: meta}
	p.AddArgumentResolver(&testArgumentResolver{
		supports: func(pm resolver.ParameterMeta) bool { return pm.Type.Kind() == reflect.Int },
		resolve:  func(ctx execution.Context, pm resolver.ParameterMeta) (any, error) { return 1, nil },
	})
	p.AddReturnValueHandler(&testReturnHandler{
		supports: func(rt reflect.Type) bool { return rt.Kind() == reflect.String },
		handle:   func(v any, ctx execution.Context) error { return nil },
	})

	if err := p.Execute(newTestExecutionContext()); err != nil {
		t.Fatalf("실행 실패: %v", err)
	}
	if got.MethodName != meta.Method.Name || got.ControllerType != meta.ControllerType {
		t.Fatalf("전역 AfterCompletion은 실제 meta를 받아야 합니다. got=%+v want=%+v", got, meta)
	}
}

func TestExecute_MissingArgumentResolverReturnsError(t *testing.T) {
	controllerCalled := 0
	p, _ := newPipelineWithController(t, "Handle", &controllerCalled)

	err := p.Execute(newTestExecutionContext())
	if err == nil {
		t.Fatal("ArgumentResolver 에러가 발생해야 합니다")
	}
	if controllerCalled != 0 {
		t.Fatalf("컨트롤러가 호출되면 안 됩니다. 실제 호출 횟수: %d", controllerCalled)
	}
}

func TestExecute_ResponsePreparationFailureSkipsPostHooks(t *testing.T) {
	controllerCalled := 0
	p, _ := newPipelineWithController(t, "Handle", &controllerCalled)

	p.AddArgumentResolver(&testArgumentResolver{
		supports: func(pm resolver.ParameterMeta) bool { return pm.Type.Kind() == reflect.Int },
		resolve:  func(ctx execution.Context, pm resolver.ParameterMeta) (any, error) { return 7, nil },
	})
	p.AddReturnValueHandler(&testReturnHandler{
		supports: func(rt reflect.Type) bool { return rt.Kind() == reflect.String },
		handle: func(v any, ctx execution.Context) error {
			return errors.New("write failed")
		},
	})

	postHook := &testPostHook{}
	p.AddPostExecutionHook(postHook)

	ctx := newTestExecutionContext()
	writer := &testResponseWriter{}
	ctx.SetResponseWriter(writer)

	err := p.Execute(ctx)
	if err == nil {
		t.Fatal("리턴 핸들러 실패는 에러여야 합니다")
	}
	if postHook.called {
		t.Fatal("응답 준비 실패 후 post hook을 실행하면 안 됩니다")
	}
}

func TestExecute_ControllerReturnedErrorReachesBeforeResponseBeforeErrorWrite(t *testing.T) {
	controllerCalled := 0
	p, _ := newPipelineWithController(t, "Fail", &controllerCalled)

	ctx := newTestExecutionContext()
	writer := &testResponseWriter{}
	ctx.SetResponseWriter(writer)

	var beforeErr error
	events := []string{}
	p.AddInterceptor(&testInterceptor{
		name:   "tx",
		events: &events,
		before: func(meta core.HandlerMeta, err error) {
			beforeErr = err
			if writer.committed {
				t.Fatal("BeforeResponse 전에 오류 응답이 작성되면 안 됩니다")
			}
		},
		after: func(meta core.HandlerMeta, err error) {
			if err == nil || err.Error() != "boom" {
				t.Fatalf("AfterCompletion은 컨트롤러 오류를 받아야 합니다: %v", err)
			}
			if !writer.committed {
				t.Fatal("AfterCompletion은 오류 응답 처리 후 실행되어야 합니다")
			}
		},
	})
	p.AddReturnValueHandler(&handler.ErrorReturnHandler{})

	err := p.Execute(ctx)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("컨트롤러가 반환한 원래 오류가 전파되어야 합니다: %v", err)
	}
	if beforeErr == nil || beforeErr.Error() != "boom" {
		t.Fatalf("BeforeResponse는 컨트롤러 오류를 받아야 합니다: %v", beforeErr)
	}
	if writer.status != 500 || writer.writes != 1 {
		t.Fatalf("오류 응답은 finalization 후 한 번 기록되어야 합니다: status=%d writes=%d", writer.status, writer.writes)
	}
}

func TestExecute_NoContentTransportErrorReachesAfterCompletion(t *testing.T) {
	controllerCalled := 0
	p, _ := newPipelineWithController(t, "Handle", &controllerCalled)
	p.AddArgumentResolver(&testArgumentResolver{
		supports: func(pm resolver.ParameterMeta) bool { return pm.Type.Kind() == reflect.Int },
		resolve:  func(execution.Context, resolver.ParameterMeta) (any, error) { return 7, nil },
	})
	p.AddReturnValueHandler(&testReturnHandler{
		supports: func(rt reflect.Type) bool { return rt.Kind() == reflect.String },
		handle: func(_ any, ctx execution.Context) error {
			return ctx.(core.HTTPResponseContext).ResponseWriter().WriteStatus(http.StatusNoContent)
		},
	})
	want := errors.New("transport write failed")
	writer := &testResponseWriter{writeErr: want}
	ctx := newTestExecutionContext()
	ctx.SetResponseWriter(writer)
	var completion error
	events := []string{}
	p.AddInterceptor(&testInterceptor{name: "probe", events: &events, after: func(_ core.HandlerMeta, err error) { completion = err }})
	if err := p.Execute(ctx); !errors.Is(err, want) {
		t.Fatalf("execution error=%v", err)
	}
	if !errors.Is(completion, want) || writer.status != http.StatusNoContent {
		t.Fatalf("completion error=%v status=%d", completion, writer.status)
	}
}

func TestExecute_RecoversErrorResponseAndAfterCompletionPanics(t *testing.T) {
	controllerCalled := 0
	p, _ := newPipelineWithController(t, "Fail", &controllerCalled)

	ctx := newTestExecutionContext()
	writer := &testResponseWriter{}
	ctx.SetResponseWriter(writer)

	errorHandlerPanic := errors.New("error handler panic")
	afterCompletionPanic := errors.New("after completion panic")
	var afterCompletionErr error
	events := []string{}
	p.AddInterceptor(&testInterceptor{
		name:   "observer",
		events: &events,
		after: func(meta core.HandlerMeta, err error) {
			afterCompletionErr = err
			panic(afterCompletionPanic)
		},
	})
	p.AddReturnValueHandler(&testReturnHandler{
		supports: func(rt reflect.Type) bool { return rt.Implements(reflect.TypeFor[error]()) },
		handle: func(v any, ctx execution.Context) error {
			panic(errorHandlerPanic)
		},
	})

	err := p.Execute(ctx)
	if !errors.Is(err, errorHandlerPanic) || !errors.Is(err, afterCompletionPanic) {
		t.Fatalf("error response와 AfterCompletion panic이 최종 오류에 포함되어야 합니다: %v", err)
	}
	if afterCompletionErr == nil || !errors.Is(afterCompletionErr, errorHandlerPanic) || !strings.Contains(afterCompletionErr.Error(), "boom") {
		t.Fatalf("AfterCompletion은 controller 오류와 error handler panic을 모두 받아야 합니다: %v", afterCompletionErr)
	}
	if writer.status != 500 || writer.writes != 1 {
		t.Fatalf("error handler panic 후 generic 500 응답을 한 번 기록해야 합니다: status=%d writes=%d", writer.status, writer.writes)
	}
}

func TestExecute_BeforeResponseErrorsFlowFromRouteToGlobal(t *testing.T) {
	controllerCalled := 0
	p, meta := newPipelineWithController(t, "Handle", &controllerCalled)

	p.AddArgumentResolver(&testArgumentResolver{
		supports: func(pm resolver.ParameterMeta) bool { return pm.Type.Kind() == reflect.Int },
		resolve:  func(ctx execution.Context, pm resolver.ParameterMeta) (any, error) { return 7, nil },
	})

	events := []string{}
	routeErr := errors.New("route finalization failed")
	routeInterceptor := &testInterceptor{name: "route", events: &events, beforeErr: routeErr}
	meta.Interceptors = []core.Interceptor{routeInterceptor}
	p.router = &testRouter{meta: meta}

	var globalExecutionErr error
	p.AddInterceptor(&testInterceptor{
		name:   "global",
		events: &events,
		before: func(meta core.HandlerMeta, err error) {
			globalExecutionErr = err
		},
	})

	err := p.Execute(newTestExecutionContext())
	if !errors.Is(err, routeErr) {
		t.Fatalf("route finalizer 오류가 최종 오류에 포함되어야 합니다: %v", err)
	}
	if !errors.Is(globalExecutionErr, routeErr) {
		t.Fatalf("global finalizer는 route finalizer 오류를 받아야 합니다: %v", globalExecutionErr)
	}
}

func TestExecute_BeforeResponseFailurePreventsSuccessWrite(t *testing.T) {
	controllerCalled := 0
	p, _ := newPipelineWithController(t, "Handle", &controllerCalled)

	p.AddArgumentResolver(&testArgumentResolver{
		supports: func(pm resolver.ParameterMeta) bool { return pm.Type.Kind() == reflect.Int },
		resolve:  func(ctx execution.Context, pm resolver.ParameterMeta) (any, error) { return 7, nil },
	})

	ctx := newTestExecutionContext()
	writer := &testResponseWriter{}
	ctx.SetResponseWriter(writer)

	events := []string{}
	commitErr := errors.New("commit failed")
	p.AddInterceptor(&testInterceptor{
		name:      "tx",
		events:    &events,
		beforeErr: commitErr,
		before: func(meta core.HandlerMeta, err error) {
			if err != nil {
				t.Fatalf("성공 실행의 BeforeResponse에는 기존 오류가 없어야 합니다: %v", err)
			}
			if writer.committed {
				t.Fatal("commit 완료 전에 성공 응답이 작성되면 안 됩니다")
			}
		},
	})

	successHandled := false
	p.AddReturnValueHandler(&testReturnHandler{
		supports: func(rt reflect.Type) bool { return rt.Kind() == reflect.String },
		handle: func(v any, ctx execution.Context) error {
			successHandled = true
			return ctx.(core.HTTPResponseContext).ResponseWriter().WriteString(200, v.(string))
		},
	})

	err := p.Execute(ctx)
	if !errors.Is(err, commitErr) {
		t.Fatalf("BeforeResponse 오류가 최종 오류로 전파되어야 합니다: %v", err)
	}
	if !successHandled {
		t.Fatal("commit 전에 성공 응답을 staging해야 합니다")
	}
	if writer.status != 500 || writer.writes != 1 {
		t.Fatalf("commit 실패는 성공 응답 대신 500이어야 합니다: status=%d writes=%d", writer.status, writer.writes)
	}
}

func TestExecute_ResponsePreparationFailureReachesBeforeResponseBeforeCommit(t *testing.T) {
	ctr := container.New()
	if err := ctr.RegisterConstructor(func() *invalidJSONController { return &invalidJSONController{} }); err != nil {
		t.Fatal(err)
	}
	controllerType := reflect.TypeFor[*invalidJSONController]()
	method, ok := controllerType.MethodByName("Handle")
	if !ok {
		t.Fatal("Handle 메서드를 찾을 수 없습니다")
	}
	p := NewPipeline(
		&testRouter{meta: execution.HandlerMeta{ControllerType: controllerType, Method: method}},
		invoker.NewInvoker(ctr),
	)
	p.AddReturnValueHandler(&handler.JSONReturnHandler{}, &handler.ErrorReturnHandler{})
	postHook := &testPostHook{}
	p.AddPostExecutionHook(postHook)

	events := []string{}
	var beforeErr error
	p.AddInterceptor(&testInterceptor{
		name:   "tx",
		events: &events,
		before: func(_ core.HandlerMeta, err error) { beforeErr = err },
	})
	ctx := newTestExecutionContext()
	writer := &testResponseWriter{}
	ctx.SetResponseWriter(writer)

	err := p.Execute(ctx)
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("JSON 직렬화 실패가 최종 오류여야 합니다: %v", err)
	}
	if beforeErr == nil || !strings.Contains(beforeErr.Error(), "unsupported type") {
		t.Fatalf("BeforeResponse는 성공이 아니라 응답 준비 실패를 받아야 합니다: %v", beforeErr)
	}
	if postHook.called {
		t.Fatal("JSON 직렬화 실패 후 domain-event post hook을 실행하면 안 됩니다")
	}
	if writer.status != http.StatusInternalServerError || writer.writes != 1 {
		t.Fatalf("실패한 성공 응답 대신 500 하나만 기록되어야 합니다: status=%d writes=%d", writer.status, writer.writes)
	}
}

func TestExecute_CookieValidationFailureReachesBeforeResponseBeforeCommit(t *testing.T) {
	ctr := container.New()
	if err := ctr.RegisterConstructor(func() *invalidCookieController { return &invalidCookieController{} }); err != nil {
		t.Fatal(err)
	}
	controllerType := reflect.TypeFor[*invalidCookieController]()
	method, ok := controllerType.MethodByName("Handle")
	if !ok {
		t.Fatal("Handle 메서드를 찾을 수 없습니다")
	}
	p := NewPipeline(
		&testRouter{meta: execution.HandlerMeta{ControllerType: controllerType, Method: method}},
		invoker.NewInvoker(ctr),
	)
	p.AddReturnValueHandler(&handler.StringReturnHandler{}, &handler.ErrorReturnHandler{})

	var beforeErr error
	p.AddInterceptor(&testInterceptor{
		name:   "tx",
		events: &[]string{},
		before: func(_ core.HandlerMeta, err error) { beforeErr = err },
	})
	ctx := newTestExecutionContext()
	writer := &testResponseWriter{}
	ctx.SetResponseWriter(writer)

	err := p.Execute(ctx)
	if err == nil || !strings.Contains(err.Error(), "invalid cookie") {
		t.Fatalf("쿠키 검증 실패가 최종 오류여야 합니다: %v", err)
	}
	if beforeErr == nil || !strings.Contains(beforeErr.Error(), "invalid cookie") {
		t.Fatalf("BeforeResponse는 응답 준비 중 쿠키 검증 실패를 받아야 합니다: %v", beforeErr)
	}
	if writer.status != http.StatusInternalServerError || writer.writes != 1 {
		t.Fatalf("실패한 성공 응답 대신 500 하나만 기록되어야 합니다: status=%d writes=%d", writer.status, writer.writes)
	}
}

func TestExecute_StagedTransportWriteFailureIsReturned(t *testing.T) {
	controllerCalled := 0
	p, _ := newPipelineWithController(t, "Handle", &controllerCalled)
	p.AddArgumentResolver(&testArgumentResolver{
		supports: func(pm resolver.ParameterMeta) bool { return pm.Type.Kind() == reflect.Int },
		resolve:  func(execution.Context, resolver.ParameterMeta) (any, error) { return 7, nil },
	})
	p.AddReturnValueHandler(&testReturnHandler{
		supports: func(rt reflect.Type) bool { return rt.Kind() == reflect.String },
		handle: func(value any, ctx execution.Context) error {
			rw := ctx.(core.HTTPResponseContext).ResponseWriter()
			return rw.WriteString(http.StatusOK, value.(string))
		},
	})
	writeErr := errors.New("socket write failed")
	writer := &testResponseWriter{writeErr: writeErr}
	ctx := newTestExecutionContext()
	ctx.SetResponseWriter(writer)

	err := p.Execute(ctx)
	if !errors.Is(err, writeErr) {
		t.Fatalf("실제 transport 쓰기 실패가 최종 오류에 보존되어야 합니다: %v", err)
	}
	if writer.writes != 1 {
		t.Fatalf("커밋된 transport 실패 뒤 이중 응답을 시도하면 안 됩니다: writes=%d", writer.writes)
	}
}

func TestHandleExecutionError_WritesHTTPError(t *testing.T) {
	p := &Pipeline{}
	ctx := newTestExecutionContext()
	writer := &testResponseWriter{}
	ctx.SetResponseWriter(writer)

	p.handleExecutionError(ctx, httperr.BadRequest("bad request"))

	if writer.writes != 1 || writer.status != 400 {
		t.Fatalf("400 응답은 한 번만 기록되어야 합니다. 실제 writes=%d status=%d", writer.writes, writer.status)
	}
	var body map[string]string
	if err := json.Unmarshal(writer.body.(json.RawMessage), &body); err != nil {
		t.Fatal(err)
	}
	if body["message"] != "bad request" {
		t.Fatalf("예상하지 못한 메시지입니다: %v", body["message"])
	}
}

func TestHandleExecutionError_WritesInternalServerError(t *testing.T) {
	p := &Pipeline{}
	ctx := newTestExecutionContext()
	writer := &testResponseWriter{}
	ctx.SetResponseWriter(writer)

	p.handleExecutionError(ctx, errors.New("boom"))

	if writer.writes != 1 || writer.status != 500 {
		t.Fatalf("500 응답은 한 번만 기록되어야 합니다. 실제 writes=%d status=%d", writer.writes, writer.status)
	}
}

func TestHandleExecutionError_InvalidStatusFallsBackAndReportsConfigurationError(t *testing.T) {
	p := &Pipeline{}
	ctx := newTestExecutionContext()
	writer := &testResponseWriter{}
	ctx.SetResponseWriter(writer)

	err := p.handleExecutionError(ctx, &httperr.HTTPError{Status: 42, Message: "invalid"})
	if err == nil || !strings.Contains(err.Error(), "invalid HTTP error status 42") {
		t.Fatalf("잘못된 status가 명시적 오류여야 합니다: %v", err)
	}
	if writer.writes != 1 || writer.status != http.StatusInternalServerError {
		t.Fatalf("잘못된 status는 안전한 500으로 대체되어야 합니다: writes=%d status=%d", writer.writes, writer.status)
	}
}

func TestHandleExecutionError_PreservesWriteFailure(t *testing.T) {
	p := &Pipeline{}
	ctx := newTestExecutionContext()
	writeErr := errors.New("write failed")
	writer := &testResponseWriter{writeErr: writeErr}
	ctx.SetResponseWriter(writer)

	err := p.handleExecutionError(ctx, errors.New("boom"))
	if !errors.Is(err, writeErr) {
		t.Fatalf("오류 응답 쓰기 실패가 보존되어야 합니다: %v", err)
	}
}

func TestHandleExecutionError_SkipsWhenCommitted(t *testing.T) {
	p := &Pipeline{}
	ctx := newTestExecutionContext()
	writer := &testResponseWriter{committed: true}
	ctx.SetResponseWriter(writer)

	p.handleExecutionError(ctx, errors.New("boom"))

	if writer.writes != 0 {
		t.Fatalf("응답이 이미 커밋된 경우 기록되면 안 됩니다. 실제 기록 횟수: %d", writer.writes)
	}
}

func TestBuildParameterMeta_AssignsPathKeysOnlyForPathTypes(t *testing.T) {
	ctx := newTestExecutionContext()
	ctx.pathKeys = []string{"id", "name"}

	method, ok := reflect.TypeOf(&pathController{}).MethodByName("Mixed")
	if !ok {
		t.Fatal("Mixed 메서드를 찾을 수 없습니다")
	}

	metas := buildParameterMeta(method, ctx.pathKeys)
	if len(metas) != 3 {
		t.Fatalf("예상하지 못한 메타 길이입니다: %d", len(metas))
	}
	if metas[0].PathKey != "id" {
		t.Fatalf("첫 번째 path key가 예상과 다릅니다: %q", metas[0].PathKey)
	}
	if metas[1].PathKey != "" {
		t.Fatalf("path 타입이 아닌 파라미터에는 path key가 없어야 합니다: %q", metas[1].PathKey)
	}
	if metas[2].PathKey != "name" {
		t.Fatalf("세 번째 path key가 예상과 다릅니다: %q", metas[2].PathKey)
	}
}

var _ hook.PostExecutionHook = (*testPostHook)(nil)
var _ handler.ReturnValueHandler = (*testReturnHandler)(nil)

func (c *testExecutionContext) SetRoute(params map[string]string, pathKeys []string) {
	c.params = params
	c.pathKeys = pathKeys
}

func (c *testExecutionContext) ResponseWriter() core.ResponseWriter          { return c.writer }
func (c *testExecutionContext) SetResponseWriter(writer core.ResponseWriter) { c.writer = writer }
