package router

import (
	"reflect"
	"strings"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/httperr"
)

type RouteOption func(*RouteSpec)

type RouteSpec struct {
	Method       string
	Path         string
	Handler      any
	Interceptors []core.Interceptor
}

type Router interface {
	Route(ctx core.ExecutionContext) (core.HandlerMeta, error)
}

type routeNode struct {
	staticChildren map[string]*routeNode
	paramChild     *routeNode
	paramKey       string
	meta           *core.HandlerMeta
}

type DefaultRouter struct {
	trees           map[string]*routeNode
	controllerTypes []reflect.Type
	seenControllers map[reflect.Type]struct{}
}

func NewRouter() *DefaultRouter {
	return &DefaultRouter{
		trees:           make(map[string]*routeNode),
		seenControllers: make(map[reflect.Type]struct{}),
	}
}

func (r *DefaultRouter) ControllerTypes() []reflect.Type {
	return append([]reflect.Type(nil), r.controllerTypes...)
}

func (r *DefaultRouter) Register(method string, path string, meta core.HandlerMeta) {
	root := r.trees[method]
	if root == nil {
		root = &routeNode{}
		r.trees[method] = root
	}

	meta.PathKeys = extractPathKeys(path)
	node := root
	for _, seg := range splitPath(path) {
		if isParamSegment(seg) {
			if node.paramChild == nil {
				node.paramChild = &routeNode{paramKey: seg[1:]}
			}
			node = node.paramChild
			continue
		}

		if node.staticChildren == nil {
			node.staticChildren = make(map[string]*routeNode)
		}
		child := node.staticChildren[seg]
		if child == nil {
			child = &routeNode{}
			node.staticChildren[seg] = child
		}
		node = child
	}

	metaCopy := meta
	node.meta = &metaCopy

	if _, ok := r.seenControllers[meta.ControllerType]; !ok {
		r.seenControllers[meta.ControllerType] = struct{}{}
		r.controllerTypes = append(r.controllerTypes, meta.ControllerType)
	}
}

func (r *DefaultRouter) Route(ctx core.ExecutionContext) (core.HandlerMeta, error) {
	root := r.trees[ctx.Method()]
	if root == nil {
		return core.HandlerMeta{}, httperr.NotFound("핸들러가 없습니다.")
	}

	pathSegs := splitPath(ctx.Path())
	node := root

	var params map[string]string
	var pathKeys []string

	for _, seg := range pathSegs {
		if node.staticChildren != nil {
			if child := node.staticChildren[seg]; child != nil {
				node = child
				continue
			}
		}

		if node.paramChild == nil {
			return core.HandlerMeta{}, httperr.NotFound("핸들러가 없습니다.")
		}

		if params == nil {
			params = make(map[string]string, len(pathSegs))
		}
		if pathKeys == nil {
			pathKeys = make([]string, 0, len(pathSegs))
		}
		params[node.paramChild.paramKey] = seg
		pathKeys = append(pathKeys, node.paramChild.paramKey)
		node = node.paramChild
	}

	if node.meta == nil {
		return core.HandlerMeta{}, httperr.NotFound("핸들러가 없습니다.")
	}

	if params != nil {
		ctx.Set("spine.params", params)
		ctx.Set("spine.pathKeys", append([]string(nil), node.meta.PathKeys...))
	}

	return *node.meta, nil
}

func matchPath(pattern string, path string) (bool, map[string]string, []string) {
	patternSegs := splitPath(pattern)
	pathSegs := splitPath(path)

	if len(patternSegs) != len(pathSegs) {
		return false, nil, nil
	}

	var params map[string]string
	var keys []string

	for i := 0; i < len(patternSegs); i++ {
		p := patternSegs[i]
		v := pathSegs[i]

		if isParamSegment(p) {
			if params == nil {
				params = make(map[string]string, len(patternSegs))
				keys = make([]string, 0, len(patternSegs))
			}
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

func extractPathKeys(path string) []string {
	segs := splitPath(path)
	keys := make([]string, 0, len(segs))
	for _, seg := range segs {
		if isParamSegment(seg) {
			keys = append(keys, seg[1:])
		}
	}
	return keys
}

func isParamSegment(seg string) bool {
	return len(seg) > 0 && seg[0] == ':'
}
