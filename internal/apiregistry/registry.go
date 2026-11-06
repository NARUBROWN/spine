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
type Registry struct {
	mu     sync.Mutex
	owner  reflect.Type
	routes []router.RouteSpec
	errors []error
	frozen bool
}

func New(owner reflect.Type) *Registry { return &Registry{owner: owner} }

// Error는 사용자 구성 오류를 앱 준비 오류에 포함합니다.
func (r *Registry) Error(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		panic("api.Registry is frozen")
	}
	if err != nil {
		r.errors = append(r.errors, err)
	}
}

func (r *Registry) add(method, path string, handler any, options []router.RouteOption) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		panic("api.Registry is frozen")
	}
	spec := router.RouteSpec{Method: method, Path: path, Handler: handler, ControllerType: r.owner}
	for _, option := range options {
		if option == nil {
			r.errors = append(r.errors, fmt.Errorf("nil route option for %s %s", method, path))
			continue
		}
		option(&spec)
	}
	spec.Interceptors = append(spec.Interceptors[:0:0], spec.Interceptors...)
	r.routes = append(r.routes, spec)
}

// Freeze는 이후 등록을 거부하고 호출자가 소유하는 스냅샷을 반환합니다.
func (r *Registry) Freeze() ([]router.RouteSpec, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frozen = true
	routes := append([]router.RouteSpec(nil), r.routes...)
	for i := range routes {
		routes[i].Interceptors = append(routes[i].Interceptors[:0:0], routes[i].Interceptors...)
	}
	return routes, errors.Join(r.errors...)
}
func (r *Registry) GET(path string, handler any, options ...router.RouteOption) {
	r.add(http.MethodGet, path, handler, options)
}
func (r *Registry) POST(path string, handler any, options ...router.RouteOption) {
	r.add(http.MethodPost, path, handler, options)
}
func (r *Registry) PUT(path string, handler any, options ...router.RouteOption) {
	r.add(http.MethodPut, path, handler, options)
}
func (r *Registry) PATCH(path string, handler any, options ...router.RouteOption) {
	r.add(http.MethodPatch, path, handler, options)
}
func (r *Registry) DELETE(path string, handler any, options ...router.RouteOption) {
	r.add(http.MethodDelete, path, handler, options)
}
func (r *Registry) HEAD(path string, handler any, options ...router.RouteOption) {
	r.add(http.MethodHead, path, handler, options)
}
func (r *Registry) OPTIONS(path string, handler any, options ...router.RouteOption) {
	r.add(http.MethodOptions, path, handler, options)
}
