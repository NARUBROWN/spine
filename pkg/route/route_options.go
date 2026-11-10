// Package route는 기존 App.Route의 옵션을 제공합니다.
//
// Deprecated: 새 HTTP 등록에는 pkg/api를 사용하세요.
package route

import (
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/router"
)

// WithInterceptors는 기존 App.Route에 라우트 인터셉터를 추가합니다.
//
// Deprecated: 새 컨트롤러와 그룹에서는 api.Interceptors를 사용하세요.
func WithInterceptors(interceptors ...core.Interceptor) router.RouteOption {
	return router.NewOption(func(rs *router.RouteSpec) {
		rs.Interceptors = append(rs.Interceptors, interceptors...)
	})
}
