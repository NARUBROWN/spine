// Package route provides options for the legacy App.Route API.
//
// Deprecated: Use pkg/api for new HTTP registrations.
package route

import (
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/router"
)

// WithInterceptors adds route interceptors to the legacy App.Route API.
//
// Deprecated: Use api.Interceptors in new controllers and groups.
func WithInterceptors(interceptors ...core.Interceptor) router.RouteOption {
	return router.NewOption(func(rs *router.RouteSpec) {
		rs.Interceptors = append(rs.Interceptors, interceptors...)
	})
}
