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

func TestNewHandlerMeta_RejectsNilWithoutPanic(t *testing.T) {
	if _, err := NewHandlerMeta(nil); err == nil {
		t.Fatal("nil handler는 오류여야 합니다")
	}
}

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

	params := ctx.Params()
	if params["id"] != "42" {
		t.Fatalf("path 파라미터 id가 잘못되었습니다: %q", params["id"])
	}

	pathKeys := ctx.PathKeys()
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

func TestRouter_DoesNotExposeCanonicalPathKeysToContext(t *testing.T) {
	r := NewRouter()
	r.Register("GET", "/users/:id", testHandlerMeta("List"))
	first := newTestExecutionContext("GET", "/users/1")
	meta, err := r.Route(first)
	if err != nil {
		t.Fatal(err)
	}
	// 이 테스트 대역은 입력 슬라이스를 그대로 보관하므로 라우터 경계를 검증합니다.
	first.pathKeys[0] = "mutated"
	second := newTestExecutionContext("GET", "/users/2")
	next, err := r.Route(second)
	if err != nil || meta.PathKeys[0] != "id" || next.PathKeys[0] != "id" || second.PathKeys()[0] != "id" {
		t.Fatalf("한 요청이 라우터 메타데이터와 다음 요청의 키를 바꾸면 안 됩니다: first=%v next=%v err=%v", meta.PathKeys, next.PathKeys, err)
	}
}

func TestRouter_BacktracksFromStaticDeadEndToParameterRoute(t *testing.T) {
	r := NewRouter()
	r.Register("GET", "/files/static/metadata", testHandlerMeta("List"))
	r.Register("GET", "/files/:name/download", testHandlerMeta("Create"))

	ctx := newTestExecutionContext("GET", "/files/static/download")
	got, err := r.Route(ctx)
	if err != nil {
		t.Fatalf("정적 분기의 dead-end 뒤 parameter route로 backtrack해야 합니다: %v", err)
	}
	if got.Method.Name != "Create" {
		t.Fatalf("parameter route가 선택되어야 합니다: %s", got.Method.Name)
	}
	params := ctx.Params()
	if params["name"] != "static" {
		t.Fatalf("backtrack한 parameter 값이 보존되어야 합니다: %#v", params)
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
	r := NewRouter()
	r.Register("GET", "/team/:teamId/user/:userId", testHandlerMeta("List"))
	ctx := newTestExecutionContext("GET", "/team/alpha/user/7")
	meta, err := r.Route(ctx)
	if err != nil {
		t.Fatalf("매칭되어야 합니다: %v", err)
	}
	params := ctx.Params()
	if params["teamId"] != "alpha" || params["userId"] != "7" {
		t.Fatalf("파라미터 매핑이 잘못되었습니다: %v", params)
	}
	keys := ctx.PathKeys()
	if len(keys) != 2 || keys[0] != "teamId" || keys[1] != "userId" {
		t.Fatalf("path key 순서가 잘못되었습니다: %v", keys)
	}
	if !reflect.DeepEqual(meta.PathKeys, keys) {
		t.Fatalf("핸들러의 PathKeys와 주입된 경로 키가 일치해야 합니다: %v, %v", meta.PathKeys, keys)
	}
}

func TestMatchPathMismatch(t *testing.T) {
	r := NewRouter()
	r.Register("GET", "/team/:id", testHandlerMeta("List"))
	ctx := newTestExecutionContext("GET", "/team")
	_, err := r.Route(ctx)
	if err == nil {
		t.Fatal("세그먼트 길이가 다르면 매칭되면 안 됩니다")
	}
	var httpErr *httperr.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Status != 404 {
		t.Fatalf("세그먼트 길이가 다르면 404 오류여야 합니다: %v", err)
	}
	if value := ctx.Params(); len(value) != 0 {
		t.Fatalf("매칭 실패 시 경로 파라미터를 주입하면 안 됩니다: %v", value)
	}
	if value := ctx.PathKeys(); len(value) != 0 {
		t.Fatalf("매칭 실패 시 경로 키를 주입하면 안 됩니다: %v", value)
	}
}

func TestRouterMapsCapturedValuesToMatchedRouteKeys(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		routes := []string{"/users/:id", "/users/:userID/posts/:postID", "/users/:owner/settings/:section"}
		if reverse {
			routes[0], routes[2] = routes[2], routes[0]
		}
		r := NewRouter()
		for _, path := range routes {
			r.Register("GET", path, testHandlerMeta("List"))
		}
		for _, tc := range []struct {
			path string
			want map[string]string
		}{
			{"/users/7", map[string]string{"id": "7"}},
			{"/users/7/posts/9", map[string]string{"userID": "7", "postID": "9"}},
			{"/users/7/settings/profile", map[string]string{"owner": "7", "section": "profile"}},
		} {
			ctx := newTestExecutionContext("GET", tc.path)
			meta, err := r.Route(ctx)
			if err != nil || !reflect.DeepEqual(ctx.Params(), tc.want) {
				t.Fatalf("reverse=%v path=%s params=%v want=%v err=%v", reverse, tc.path, ctx.Params(), tc.want, err)
			}
			for _, key := range meta.PathKeys {
				if _, ok := ctx.Params()[key]; !ok {
					t.Fatalf("missing matched route key %q", key)
				}
			}
		}
	}
}

func (c *testExecutionContext) SetRoute(params map[string]string, pathKeys []string) {
	c.params = params
	c.pathKeys = pathKeys
}
