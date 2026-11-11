package router

import (
	"errors"
	"net/url"
	"reflect"
	"strings"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/execution"
)

type RouteOption struct {
	apply func(*RouteSpec)
}

func NewOption(apply func(*RouteSpec)) RouteOption { return RouteOption{apply: apply} }

func ApplyOption(option RouteOption, spec *RouteSpec) bool {
	if option.apply == nil {
		return false
	}
	option.apply(spec)
	return true
}

var (
	ErrRouteNotFound       = errors.New("handler not found")
	ErrInvalidPathEncoding = errors.New("invalid path encoding")
)

type RouteSpec struct {
	Method         string
	Path           string
	Handler        any
	ControllerType reflect.Type
	Interceptors   []core.Interceptor
	Tags           []string
	Summary        string
	Description    string
}

type Router interface {
	Route(ctx execution.Context) (execution.HandlerMeta, error)
}

type routeNode struct {
	staticChildren map[string]*routeNode
	paramChild     *routeNode
	meta           *execution.HandlerMeta
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

func (r *DefaultRouter) Register(method string, path string, meta execution.HandlerMeta) {
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
				node.paramChild = &routeNode{}
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

func (r *DefaultRouter) Route(ctx execution.Context) (execution.HandlerMeta, error) {
	root := r.trees[ctx.Method()]
	if root == nil {
		return execution.HandlerMeta{}, ErrRouteNotFound
	}

	pathSegs := splitPath(ctx.Path())
	if raw, ok := ctx.(interface{ RawPath() string }); ok {
		pathSegs = splitPath(raw.RawPath())
		for i, segment := range pathSegs {
			decoded, err := url.PathUnescape(segment)
			if err != nil {
				return execution.HandlerMeta{}, ErrInvalidPathEncoding
			}
			pathSegs[i] = decoded
		}
	}
	node, values, ok := findRoute(root, pathSegs, 0, nil)
	if !ok {
		return execution.HandlerMeta{}, ErrRouteNotFound
	}

	if len(node.meta.PathKeys) > 0 {
		params := make(map[string]string, len(node.meta.PathKeys))
		for i, key := range node.meta.PathKeys {
			params[key] = values[i]
		}
		ctx.SetRoute(params, append([]string(nil), node.meta.PathKeys...))
	}

	return *node.meta, nil
}

func findRoute(node *routeNode, segments []string, index int, values []string) (*routeNode, []string, bool) {
	if index == len(segments) {
		if node.meta == nil {
			return nil, nil, false
		}
		return node, values, true
	}

	segment := segments[index]
	if node.staticChildren != nil {
		if child := node.staticChildren[segment]; child != nil {
			if matched, matchedValues, ok := findRoute(child, segments, index+1, values); ok {
				return matched, matchedValues, true
			}
		}
	}

	if node.paramChild == nil {
		return nil, nil, false
	}

	return findRoute(node.paramChild, segments, index+1, append(values, segment))
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
