package apiregistry

import (
	"errors"
	"reflect"
	"testing"
)

func TestRegistryFreezesOwnedSnapshotsAndConfigurationErrors(t *testing.T) {
	r, collector := New(reflect.TypeFor[*struct{}]())
	r.GET("/first", func() {})
	r.POST("/second", func() {})
	missing := errors.New("missing configuration")
	r.Error(missing)
	routes, err := collector.Freeze(r)
	if !errors.Is(err, missing) || len(routes) != 2 {
		t.Fatal(routes, err)
	}
	routes[0].Path = "changed"
	another, _ := collector.Freeze(r)
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

func TestOverwritingRegistryPreservesCollectorError(t *testing.T) {
	registry, collector := New(nil)
	registered := errors.New("invalid configuration")
	registry.Error(registered)
	*registry = Registry{}
	routes, err := collector.Freeze(registry)
	if len(routes) != 0 || !errors.Is(err, registered) {
		t.Fatal(routes, err)
	}
}
