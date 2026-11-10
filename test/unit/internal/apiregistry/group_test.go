package apiregistry

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/core"
	contextapi "github.com/NARUBROWN/spine/pkg/spine"
)

func TestGroupsComposeAndFreeze(t *testing.T) {
	r := New(nil)
	outer := r.Group("/admin", Tags("Admin"))
	users := outer.Group("/users", Tags("Users", "Admin"))
	users.GET("/", func() {}, Summary("List"), Tags("Extra"))
	users.GET("/:id", func() {})
	routes, err := Freeze(r)
	if err != nil {
		t.Fatal(err)
	}
	if routes[0].Path != "/admin/users" || routes[1].Path != "/admin/users/:id" {
		t.Fatalf("paths: %#v", routes)
	}
	if !reflect.DeepEqual(routes[0].Tags, []string{"Admin", "Users", "Extra"}) || routes[0].Summary != "List" {
		t.Fatalf("metadata: %#v", routes[0])
	}
	routes[0].Tags[0] = "changed"
	if routes[1].Tags[0] != "Admin" {
		t.Fatal("shared tags")
	}
	defer func() {
		if recover() == nil {
			t.Error("registration after freeze should panic")
		}
	}()
	users.GET("/", func() {})
}
func TestInvalidGroupPath(t *testing.T) {
	r := New(nil)
	r.Group("users").GET("/", func() {})
	if _, err := Freeze(r); err == nil {
		t.Fatal("expected error")
	}
}

type groupInterceptor struct{ name string }

func (*groupInterceptor) PreHandle(contextapi.Ctx, core.HandlerMeta) error { return nil }
func (*groupInterceptor) PostHandle(contextapi.Ctx, core.HandlerMeta)      {}
func (*groupInterceptor) BeforeResponse(contextapi.Ctx, core.HandlerMeta, error) error {
	return nil
}
func (*groupInterceptor) AfterCompletion(contextapi.Ctx, core.HandlerMeta, error) {}
func TestGroupInterceptorsAreInheritedInOrder(t *testing.T) {
	outer, inner, local := &groupInterceptor{"outer"}, &groupInterceptor{"inner"}, &groupInterceptor{"local"}
	r := New(nil)
	parent := r.Group("/users", Interceptors(outer))
	nested := parent.Group("/admin", Interceptors(inner))
	nested.GET("/", func() {}, Interceptors(local))
	routes, err := Freeze(r)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(routes[0].Interceptors, []core.Interceptor{outer, inner, local}) {
		t.Fatalf("interceptors=%#v", routes[0].Interceptors)
	}
}

func TestRegistryAndGroupShareRouteOptions(t *testing.T) {
	r := New(nil)
	r.GET("/direct", func() {}, Tags("Direct"))
	r.Group("/group").GET("/", func() {}, Tags("Grouped"))
	routes, err := Freeze(r)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(routes[0].Tags, []string{"Direct"}) || !reflect.DeepEqual(routes[1].Tags, []string{"Grouped"}) {
		t.Fatalf("routes=%#v", routes)
	}
}

func TestGroupMethodsUseRegistryArgumentOrder(t *testing.T) {
	r := New(nil)
	group := r.Group("/group")
	handler := func() {}
	group.GET("/get", handler)
	group.POST("/post", handler)
	group.PUT("/put", handler)
	group.PATCH("/patch", handler)
	group.DELETE("/delete", handler)
	group.HEAD("/head", handler)
	group.OPTIONS("/options", handler)
	routes, err := Freeze(r)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions}
	for i, method := range want {
		if routes[i].Method != method {
			t.Fatalf("routes[%d].Method=%q want=%q", i, routes[i].Method, method)
		}
	}
}
