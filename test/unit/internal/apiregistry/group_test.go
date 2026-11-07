package apiregistry

import (
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/core"
)

func TestGroupsComposeAndFreeze(t *testing.T) {
	r := New(nil)
	outer := r.Group("/admin", Tags("Admin"))
	users := outer.Group("/users", Tags("Users", "Admin"))
	users.GET("/", Options(Summary("List"), Tags("Extra")), func() {})
	users.GET("/:id", Options(), func() {})
	routes, err := r.Freeze()
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
	users.GET("/", Options(), func() {})
}
func TestInvalidGroupPath(t *testing.T) {
	r := New(nil)
	r.Group("users").GET("/", Options(), func() {})
	if _, err := r.Freeze(); err == nil {
		t.Fatal("expected error")
	}
}

type groupInterceptor struct{ name string }

func (*groupInterceptor) PreHandle(core.ExecutionContext, core.HandlerMeta) error { return nil }
func (*groupInterceptor) PostHandle(core.ExecutionContext, core.HandlerMeta)      {}
func (*groupInterceptor) BeforeResponse(core.ExecutionContext, core.HandlerMeta, error) error {
	return nil
}
func (*groupInterceptor) AfterCompletion(core.ExecutionContext, core.HandlerMeta, error) {}
func TestGroupInterceptorsAreInheritedInOrder(t *testing.T) {
	outer, inner, local := &groupInterceptor{"outer"}, &groupInterceptor{"inner"}, &groupInterceptor{"local"}
	r := New(nil)
	parent := r.Group("/users", Interceptors(outer))
	nested := parent.Group("/admin", Interceptors(inner))
	nested.GET("/", Options(Interceptors(local)), func() {})
	routes, err := r.Freeze()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(routes[0].Interceptors, []core.Interceptor{outer, inner, local}) {
		t.Fatalf("interceptors=%#v", routes[0].Interceptors)
	}
}
