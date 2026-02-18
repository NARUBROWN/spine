package router

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/NARUBROWN/spine/pkg/httperr"
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

func newTestExecutionContext(method string, path string) *testExecutionContext {
	return &testExecutionContext{
		method:   method,
		path:     path,
		params:   map[string]string{},
		queries:  map[string][]string{},
		headers:  map[string]string{},
		store:    map[string]any{},
		pathKeys: []string{},
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

type testController struct{}

func (c *testController) List() string   { return "list" }
func (c *testController) Create() string { return "create" }

type anotherController struct{}

func (c *anotherController) Another() string { return "another" }

func testHandlerMeta(method string) core.HandlerMeta {
	ctrlType := reflect.TypeOf(&testController{})
	m, ok := ctrlType.MethodByName(method)
	if !ok {
		panic("메서드가 없습니다")
	}

	return core.HandlerMeta{
		ControllerType: ctrlType,
		Method:         m,
	}
}

func anotherHandlerMeta() core.HandlerMeta {
	ctrlType := reflect.TypeOf(&anotherController{})
	m, ok := ctrlType.MethodByName("Another")
	if !ok {
		panic("메서드가 없습니다")
	}

	return core.HandlerMeta{
		ControllerType: ctrlType,
		Method:         m,
	}
}

func TestRouter_RouteMatchesPathAndInjectsPathArgs(t *testing.T) {
	r := NewRouter()
	meta := testHandlerMeta("List")
	r.Register("GET", "/users/:id", meta)

	ctx := newTestExecutionContext("GET", "/users/42")
	got, err := r.Route(ctx)
	if err != nil {
		t.Fatalf("라우팅 실패했습니다: %v", err)
	}
	if got.ControllerType != meta.ControllerType {
		t.Fatalf("예상한 컨트롤러 타입과 일치하지 않습니다")
	}
	if got.Method.Name != meta.Method.Name {
		t.Fatalf("예상한 메서드와 일치하지 않습니다: %s", got.Method.Name)
	}

	paramsAny, ok := ctx.Get("spine.params")
	if !ok {
		t.Fatal("spine.params가 컨텍스트에 주입되지 않았습니다")
	}
	params, ok := paramsAny.(map[string]string)
	if !ok {
		t.Fatalf("spine.params 타입이 잘못되었습니다: %T", paramsAny)
	}
	if params["id"] != "42" {
		t.Fatalf("path 파라미터 id가 잘못되었습니다: %q", params["id"])
	}

	pathKeysAny, ok := ctx.Get("spine.pathKeys")
	if !ok {
		t.Fatal("spine.pathKeys가 컨텍스트에 주입되지 않았습니다")
	}
	pathKeys, ok := pathKeysAny.([]string)
	if !ok {
		t.Fatalf("spine.pathKeys 타입이 잘못되었습니다: %T", pathKeysAny)
	}
	if len(pathKeys) != 1 || pathKeys[0] != "id" {
		t.Fatalf("pathKeys 값이 잘못되었습니다: %v", pathKeys)
	}
}

func TestRouter_RouteRequiresMethodAndPathMatch(t *testing.T) {
	r := NewRouter()
	r.Register("POST", "/users/:id", testHandlerMeta("Create"))

	ctx := newTestExecutionContext("GET", "/users/42")
	_, err := r.Route(ctx)
	if err == nil {
		t.Fatal("메서드가 맞지 않으면 핸들러를 찾지 못해야 합니다")
	}

	var httpErr *httperr.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("httperr HTTPError가 예상됐지만 실제: %v", err)
	}
	if httpErr.Status != 404 {
		t.Fatalf("NotFound 상태 코드는 404여야 합니다: %d", httpErr.Status)
	}
}

func TestRouter_ControllerTypesDeduplicatesControllers(t *testing.T) {
	r := NewRouter()
	r.Register("GET", "/one", testHandlerMeta("List"))
	r.Register("GET", "/two", testHandlerMeta("Create"))
	r.Register("GET", "/again", testHandlerMeta("List"))
	r.Register("GET", "/third", anotherHandlerMeta())

	types := r.ControllerTypes()
	if len(types) != 2 {
		t.Fatalf("중복 제거 후 타입은 2개여야 합니다: %d", len(types))
	}

	count := 0
	typeA := reflect.TypeOf(&testController{})
	typeB := reflect.TypeOf(&anotherController{})
	for _, tpe := range types {
		if tpe == typeA || tpe == typeB {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("예상 컨트롤러 타입이 누락되었습니다: %v", types)
	}
}

func TestSplitPath(t *testing.T) {
	if got := splitPath("/"); len(got) != 0 {
		t.Fatalf("루트 경로는 빈 세그먼트를 반환해야 합니다: %v", got)
	}
	if got := splitPath("/users/42/"); len(got) != 2 || got[0] != "users" || got[1] != "42" {
		t.Fatalf("슬래시 정리 로직이 잘못되었습니다: %v", got)
	}
	if got := splitPath("users/42"); len(got) != 2 || got[1] != "42" {
		t.Fatalf("앞 슬래시 없는 경로 처리 결과가 잘못되었습니다: %v", got)
	}
}

func TestMatchPathWithParams(t *testing.T) {
	ok, params, keys := matchPath("/team/:teamId/user/:userId", "/team/alpha/user/7")
	if !ok {
		t.Fatal("매칭되어야 합니다")
	}
	if params["teamId"] != "alpha" || params["userId"] != "7" {
		t.Fatalf("파라미터 매핑이 잘못되었습니다: %v", params)
	}
	if len(keys) != 2 || keys[0] != "teamId" || keys[1] != "userId" {
		t.Fatalf("path key 순서가 잘못되었습니다: %v", keys)
	}
}

func TestMatchPathMismatch(t *testing.T) {
	ok, _, _ := matchPath("/team/:id", "/team")
	if ok {
		t.Fatal("세그먼트 길이가 다르면 매칭되면 안 됩니다")
	}
}
