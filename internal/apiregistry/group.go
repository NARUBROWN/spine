package apiregistry

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/router"
)

type Option = router.RouteOption

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
func (g *Group) add(method, path string, handler any, options []Option) {
	if path == "" || !strings.HasPrefix(path, "/") || strings.Contains(path, "//") {
		g.registry.Error(fmt.Errorf("invalid group route path %q", path))
		return
	}
	all := append(append([]Option(nil), g.options...), options...)
	g.registry.add(method, join(g.prefix, path), handler, all)
}
func (g *Group) GET(p string, h any, options ...Option) { g.add(http.MethodGet, p, h, options) }
func (g *Group) POST(p string, h any, options ...Option) {
	g.add(http.MethodPost, p, h, options)
}
func (g *Group) PUT(p string, h any, options ...Option) { g.add(http.MethodPut, p, h, options) }
func (g *Group) PATCH(p string, h any, options ...Option) {
	g.add(http.MethodPatch, p, h, options)
}
func (g *Group) DELETE(p string, h any, options ...Option) {
	g.add(http.MethodDelete, p, h, options)
}
func (g *Group) HEAD(p string, h any, options ...Option) { g.add(http.MethodHead, p, h, options) }
func (g *Group) OPTIONS(p string, h any, options ...Option) {
	g.add(http.MethodOptions, p, h, options)
}
