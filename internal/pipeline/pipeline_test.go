package pipeline

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/event/hook"
	"github.com/NARUBROWN/spine/internal/handler"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/path"
)

type testEventBus struct{}

func (b *testEventBus) Publish(events ...publish.DomainEvent) {}
func (b *testEventBus) Drain() []publish.DomainEvent          { return nil }

type testExecutionContext struct {
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
func (c *testExecutionContext) EventBus() core.EventBus      { return &testEventBus{} }
func (c *testExecutionContext) Method() string               { return c.method }
func (c *testExecutionContext) Path() string                 { return c.path }
func (c *testExecutionContext) Params() map[string]string    { return c.params }
func (c *testExecutionContext) Header(name string) string    { return c.headers[name] }
func (c *testExecutionContext) PathKeys() []string           { return c.pathKeys }
func (c *testExecutionContext) Queries() map[string][]string { return c.queries }
func (c *testExecutionContext) Set(key string, value any)    { c.store[key] = value }
func (c *testExecutionContext) Get(key string) (any, bool)   { v, ok := c.store[key]; return v, ok }

type testRouter struct {
	meta core.HandlerMeta
	err  error
}

func (r *testRouter) Route(ctx core.ExecutionContext) (core.HandlerMeta, error) {
	if r.err != nil {
		return core.HandlerMeta{}, r.err
	}
	return r.meta, nil
}

type testInterceptor struct {
	name   string
	events *[]string
	preErr error
}

func (i *testInterceptor) PreHandle(ctx core.ExecutionContext, meta core.HandlerMeta) error {
	*i.events = append(*i.events, "pre:"+i.name)
	return i.preErr
}
func (i *testInterceptor) PostHandle(ctx core.ExecutionContext, meta core.HandlerMeta) {
	*i.events = append(*i.events, "post:"+i.name)
}
func (i *testInterceptor) AfterCompletion(ctx core.ExecutionContext, meta core.HandlerMeta, err error) {
	*i.events = append(*i.events, "after:"+i.name)
}

type testArgumentResolver struct {
	supports func(pm resolver.ParameterMeta) bool
	resolve  func(ctx core.ExecutionContext, pm resolver.ParameterMeta) (any, error)
}

func (r *testArgumentResolver) Supports(pm resolver.ParameterMeta) bool {
	return r.supports(pm)
}
func (r *testArgumentResolver) Resolve(ctx core.ExecutionContext, pm resolver.ParameterMeta) (any, error) {
	return r.resolve(ctx, pm)
}

type testReturnHandler struct {
	supports func(rt reflect.Type) bool
	handle   func(v any, ctx core.ExecutionContext) error
}

func (h *testReturnHandler) Supports(rt reflect.Type) bool {
	return h.supports(rt)
}
func (h *testReturnHandler) Handle(v any, ctx core.ExecutionContext) error {
	return h.handle(v, ctx)
}

type testPostHook struct {
	called  bool
	results []any
	err     error
}

func (h *testPostHook) AfterExecution(ctx core.ExecutionContext, result []any, err error) {
	h.called = true
	h.results = result
	h.err = err
}

type testResponseWriter struct {
	committed bool
	status    int
	body      any
	writes    int
}

func (w *testResponseWriter) SetHeader(key, value string) {}
func (w *testResponseWriter) AddHeader(key, value string) {}
func (w *testResponseWriter) IsCommitted() bool           { return w.committed }
func (w *testResponseWriter) WriteStatus(status int) error {
	w.committed = true
	w.status = status
	w.writes++
	return nil
}
func (w *testResponseWriter) WriteJSON(status int, value any) error {
	w.committed = true
	w.status = status
	w.body = value
	w.writes++
	return nil
}
func (w *testResponseWriter) WriteString(status int, value string) error {
	w.committed = true
	w.status = status
	w.body = value
	w.writes++
	return nil
}
func (w *testResponseWriter) WriteBytes(status int, value []byte) error {
	w.committed = true
	w.status = status
	w.body = value
	w.writes++
	return nil
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

type pathController struct{}

func (c *pathController) Mixed(id path.Int, count int, name path.String) {}

func newPipelineWithController(t *testing.T, methodName string, called *int) (*Pipeline, core.HandlerMeta) {
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

	meta := core.HandlerMeta{
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
		resolve:  func(ctx core.ExecutionContext, pm resolver.ParameterMeta) (any, error) { return 7, nil },
	})

	handled := false
	p.AddReturnValueHandler(&testReturnHandler{
		supports: func(rt reflect.Type) bool { return rt.Kind() == reflect.String },
		handle: func(v any, ctx core.ExecutionContext) error {
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

	expected := []string{"pre:global", "pre:route", "post:route", "post:global", "after:route", "after:global"}
	if len(events) != len(expected) {
		t.Fatalf("예상하지 못한 인터셉터 이벤트 개수입니다: %v", events)
	}
	for i := range expected {
		if events[i] != expected[i] {
			t.Fatalf("이벤트 순서가 예상과 다릅니다 (인덱스 %d): 실제 %s, 기대 %s", i, events[i], expected[i])
		}
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

	p.AddArgumentResolver(&testArgumentResolver{
		supports: func(pm resolver.ParameterMeta) bool { return pm.Type.Kind() == reflect.Int },
		resolve:  func(ctx core.ExecutionContext, pm resolver.ParameterMeta) (any, error) { return 1, nil },
	})

	err := p.Execute(newTestExecutionContext())
	if err != nil {
		t.Fatalf("실행은 에러 없이 중단되어야 합니다. 실제 에러: %v", err)
	}
	if controllerCalled != 0 {
		t.Fatalf("중단 시 컨트롤러가 호출되면 안 됩니다. 실제 호출 횟수: %d", controllerCalled)
	}

	expected := []string{"pre:global", "pre:route", "after:route", "after:global"}
	if len(events) != len(expected) {
		t.Fatalf("예상하지 못한 인터셉터 이벤트 개수입니다: %v", events)
	}
	for i := range expected {
		if events[i] != expected[i] {
			t.Fatalf("이벤트 순서가 예상과 다릅니다 (인덱스 %d): 실제 %s, 기대 %s", i, events[i], expected[i])
		}
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

func TestHandleExecutionError_WritesHTTPError(t *testing.T) {
	p := &Pipeline{}
	ctx := newTestExecutionContext()
	writer := &testResponseWriter{}
	ctx.Set("spine.response_writer", writer)

	p.handleExecutionError(ctx, httperr.BadRequest("bad request"))

	if writer.writes != 1 || writer.status != 400 {
		t.Fatalf("400 응답은 한 번만 기록되어야 합니다. 실제 writes=%d status=%d", writer.writes, writer.status)
	}
	body, ok := writer.body.(map[string]any)
	if !ok {
		t.Fatalf("예상하지 못한 바디 타입입니다: %T", writer.body)
	}
	if body["message"] != "bad request" {
		t.Fatalf("예상하지 못한 메시지입니다: %v", body["message"])
	}
}

func TestHandleExecutionError_WritesInternalServerError(t *testing.T) {
	p := &Pipeline{}
	ctx := newTestExecutionContext()
	writer := &testResponseWriter{}
	ctx.Set("spine.response_writer", writer)

	p.handleExecutionError(ctx, errors.New("boom"))

	if writer.writes != 1 || writer.status != 500 {
		t.Fatalf("500 응답은 한 번만 기록되어야 합니다. 실제 writes=%d status=%d", writer.writes, writer.status)
	}
}

func TestHandleExecutionError_SkipsWhenCommitted(t *testing.T) {
	p := &Pipeline{}
	ctx := newTestExecutionContext()
	writer := &testResponseWriter{committed: true}
	ctx.Set("spine.response_writer", writer)

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

	metas := buildParameterMeta(method, ctx)
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
