package apiregistry

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/router"
)

type Option func(*router.RouteSpec)
type OptionSet struct{ values []Option }

func Options(values ...Option) OptionSet { return OptionSet{values: append([]Option(nil), values...)} }
func Tags(tags ...string) Option {
	copyTags := append([]string(nil), tags...)
	return func(s *router.RouteSpec) {
		for _, tag := range copyTags {
			found := false
			for _, prior := range s.Tags {
				if prior == tag {
					found = true
					break
				}
			}
			if !found {
				s.Tags = append(s.Tags, tag)
			}
		}
	}
}
func Summary(value string) Option     { return func(s *router.RouteSpec) { s.Summary = value } }
func Description(value string) Option { return func(s *router.RouteSpec) { s.Description = value } }
func Interceptors(values ...core.Interceptor) Option {
	copied := append([]core.Interceptor(nil), values...)
	return func(s *router.RouteSpec) { s.Interceptors = append(s.Interceptors, copied...) }
}

type Group struct {
	registry *Registry
	prefix   string
	options  []Option
}

func (r *Registry) Group(prefix string, options ...Option) *Group {
	return (&Group{registry: r}).Group(prefix, options...)
}
func (g *Group) Group(prefix string, options ...Option) *Group {
	if prefix == "" || !strings.HasPrefix(prefix, "/") || strings.Contains(prefix, "//") {
		g.registry.Error(fmt.Errorf("invalid group prefix %q", prefix))
	}
	return &Group{registry: g.registry, prefix: join(g.prefix, prefix), options: append(append([]Option(nil), g.options...), options...)}
}
func join(prefix, path string) string {
	if path == "/" || path == "" {
		if prefix == "" {
			return "/"
		}
		return prefix
	}
	return strings.TrimSuffix(prefix, "/") + path
}
func (g *Group) add(method, path string, options OptionSet, handler any) {
	if path == "" || !strings.HasPrefix(path, "/") || strings.Contains(path, "//") {
		g.registry.Error(fmt.Errorf("invalid group route path %q", path))
		return
	}
	all := append(append([]Option(nil), g.options...), options.values...)
	converted := make([]router.RouteOption, 0, len(all))
	for _, option := range all {
		if option == nil {
			g.registry.Error(fmt.Errorf("nil group route option"))
			continue
		}
		converted = append(converted, router.RouteOption(option))
	}
	g.registry.add(method, join(g.prefix, path), handler, converted)
}
func (g *Group) GET(p string, o OptionSet, h any)     { g.add(http.MethodGet, p, o, h) }
func (g *Group) POST(p string, o OptionSet, h any)    { g.add(http.MethodPost, p, o, h) }
func (g *Group) PUT(p string, o OptionSet, h any)     { g.add(http.MethodPut, p, o, h) }
func (g *Group) PATCH(p string, o OptionSet, h any)   { g.add(http.MethodPatch, p, o, h) }
func (g *Group) DELETE(p string, o OptionSet, h any)  { g.add(http.MethodDelete, p, o, h) }
func (g *Group) HEAD(p string, o OptionSet, h any)    { g.add(http.MethodHead, p, o, h) }
func (g *Group) OPTIONS(p string, o OptionSet, h any) { g.add(http.MethodOptions, p, o, h) }
