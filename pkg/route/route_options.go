package route

import (
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/router"
)

func WithInterceptors(interceptors ...core.Interceptor) router.RouteOption {
	return func(rs *router.RouteSpec) {
		rs.Interceptors = append(rs.Interceptors, interceptors...)
	}
}
