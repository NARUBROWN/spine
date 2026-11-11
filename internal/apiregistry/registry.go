package apiregistry

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sync"

	"github.com/NARUBROWN/spine/internal/router"
)

// Registry는 컨트롤러 한 개의 등록을 수집한 뒤 고정합니다.
type Registry struct{ collector *Collector }

// Collector는 경로 등록 상태를 보관하고 준비 단계에서 마감합니다.
type Collector struct {
	mu     sync.Mutex
	owner  reflect.Type
	routes []router.RouteSpec
	errors []error
	frozen bool
}

func New(owner reflect.Type) (*Registry, *Collector) {
	collector := &Collector{owner: owner}
	return &Registry{collector: collector}, collector
}

func (registry *Registry) activeCollector() *Collector {
	if registry == nil || registry.collector == nil {
		panic("api.Registry is not initialized")
	}
	return registry.collector
}

// Error는 사용자 구성 오류를 앱 준비 오류에 포함합니다.
func (registry *Registry) Error(err error) {
	r := registry.activeCollector()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		panic("api.Registry is frozen")
	}
	if err != nil {
		r.errors = append(r.errors, err)
	}
}

func (registry *Registry) add(method, path string, handler any, options []router.RouteOption) {
	r := registry.activeCollector()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		panic("api.Registry is frozen")
	}
	spec := router.RouteSpec{Method: method, Path: path, Handler: handler, ControllerType: r.owner}
	for _, option := range options {
		if !router.ApplyOption(option, &spec) {
			r.errors = append(r.errors, fmt.Errorf("empty route option for %s %s", method, path))
			continue
		}
	}
	spec.Interceptors = append(spec.Interceptors[:0:0], spec.Interceptors...)
	spec.Tags = append([]string(nil), spec.Tags...)
	r.routes = append(r.routes, spec)
}

// Freeze는 이후 등록을 거부하고 호출자가 소유하는 스냅샷을 반환합니다.
func (r *Collector) Freeze(registry *Registry) ([]router.RouteSpec, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frozen = true
	if registry.collector != r {
		return nil, errors.Join(errors.Join(r.errors...), errors.New("api.Registry was overwritten during registration"))
	}
	routes := append([]router.RouteSpec(nil), r.routes...)
	for i := range routes {
		routes[i].Interceptors = append(routes[i].Interceptors[:0:0], routes[i].Interceptors...)
		routes[i].Tags = append([]string(nil), routes[i].Tags...)
	}
	return routes, errors.Join(r.errors...)
}
func (r *Registry) GET(path string, handler any, options ...Option) {
	r.add(http.MethodGet, path, handler, options)
}
func (r *Registry) POST(path string, handler any, options ...Option) {
	r.add(http.MethodPost, path, handler, options)
}
func (r *Registry) PUT(path string, handler any, options ...Option) {
	r.add(http.MethodPut, path, handler, options)
}
func (r *Registry) PATCH(path string, handler any, options ...Option) {
	r.add(http.MethodPatch, path, handler, options)
}
func (r *Registry) DELETE(path string, handler any, options ...Option) {
	r.add(http.MethodDelete, path, handler, options)
}
func (r *Registry) HEAD(path string, handler any, options ...Option) {
	r.add(http.MethodHead, path, handler, options)
}
func (r *Registry) OPTIONS(path string, handler any, options ...Option) {
	r.add(http.MethodOptions, path, handler, options)
}
