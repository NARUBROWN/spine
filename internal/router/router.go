package router

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/NARUBROWN/spine/core"
)

type RouteOption func(*RouteSpec)

/*
RouteSpec은 외부 프로토콜과 내부 핸들러 메서드를 명시적으로 연결하는 선언 정보

이 구조체는 부트스트랩(부팅) 단계에서 수집되며
Listen() 시점에 실제 라우터/실행 모델로 해석됨
*/
type RouteSpec struct {
	// 외부 요청의 메서드
	Method string
	// 외부 요청 경로
	Path string
	// 컨트롤러 메서드에 대한 참조
	Handler any
	// 라우터 인터셉터들
	Interceptors []core.Interceptor
}

type Route struct {
	Method string
	Path   string
	Meta   core.HandlerMeta
}

type Router interface {
	Route(ctx core.ExecutionContext) (core.HandlerMeta, error)
}

type DefaultRouter struct {
	routes []Route
}

func NewRouter() *DefaultRouter {
	return &DefaultRouter{}
}

func (r *DefaultRouter) ControllerTypes() []reflect.Type {
	seen := map[reflect.Type]struct{}{}
	var result []reflect.Type

	for _, route := range r.routes {
		t := route.Meta.ControllerType
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		result = append(result, t)
	}

	return result
}

func (r *DefaultRouter) Register(method string, path string, meta core.HandlerMeta) {
	r.routes = append(r.routes, Route{
		Method: method,
		Path:   path,
		Meta:   meta,
	})
}

func (r *DefaultRouter) Route(ctx core.ExecutionContext) (core.HandlerMeta, error) {
	for _, route := range r.routes {
		if route.Method != ctx.Method() {
			continue
		}

		ok, params, keys := matchPath(route.Path, ctx.Path())
		if !ok {
			continue
		}

		// path param 주입
		ctx.Set("spine.params", params)
		ctx.Set("spine.pathKeys", keys)

		return route.Meta, nil
	}
	return core.HandlerMeta{}, fmt.Errorf("핸들러가 없습니다.")
}

func matchPath(pattern string, path string) (bool, map[string]string, []string) {
	patternSegs := splitPath(pattern)
	pathSegs := splitPath(path)

	if len(patternSegs) != len(pathSegs) {
		return false, nil, nil
	}

	params := make(map[string]string)
	keys := make([]string, 0)

	for i := 0; i < len(patternSegs); i++ {
		p := patternSegs[i]
		v := pathSegs[i]

		if len(p) > 0 && p[0] == ':' {
			// :id 형태
			key := p[1:]
			params[key] = v
			keys = append(keys, key)
			continue
		}

		if p != v {
			return false, nil, nil
		}
	}

	return true, params, keys
}

func splitPath(path string) []string {
	if path == "" || path == "/" {
		return []string{}
	}

	// 앞뒤 슬래시 제거
	if path[0] == '/' {
		path = path[1:]
	}

	if len(path) > 0 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}

	if path == "" {
		return []string{}
	}

	return strings.Split(path, "/")
}
