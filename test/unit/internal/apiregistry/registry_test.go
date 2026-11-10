package apiregistry

import (
	"errors"
	"reflect"
	"testing"
)

func TestRegistryFreezesOwnedSnapshotsAndConfigurationErrors(t *testing.T) {
	r := New(reflect.TypeFor[*struct{}]())
	r.GET("/first", func() {})
	r.POST("/second", func() {})
	missing := errors.New("missing configuration")
	r.Error(missing)
	routes, err := Freeze(r)
	if !errors.Is(err, missing) || len(routes) != 2 {
		t.Fatal(routes, err)
	}
	routes[0].Path = "changed"
	another, _ := Freeze(r)
	if another[0].Path != "/first" {
		t.Fatal("snapshot aliases internal routes")
	}
	defer func() {
		if recover() == nil {
			t.Error("registration after freeze succeeded")
		}
	}()
	r.GET("/late", func() {})
}
