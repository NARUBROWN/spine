package spine

import (
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/bootstrap"
	"github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type App interface {
	// 생성자 선언
	Constructor(constructors ...any)
	// 라우트 선언
	Route(method string, path string, handler any)
	// 인터셉터 선언
	Interceptor(interceptors ...core.Interceptor)
	// HTTP Transport 확장 (Echo 등)
	Transport(fn func(any))
	// 실행
	Run(opts boot.Options) error
}

type app struct {
	constructors   []any
	routes         []router.RouteSpec
	interceptors   []core.Interceptor
	transportHooks []func(any)
}

func New() App {
	return &app{}
}

func (a *app) Constructor(constructors ...any) {
	a.constructors = append(a.constructors, constructors...)
}

func (a *app) Route(method string, path string, handler any) {
	a.routes = append(a.routes, router.RouteSpec{
		Method:  method,
		Path:    path,
		Handler: handler,
	})
}

func (a *app) Interceptor(interceptors ...core.Interceptor) {
	a.interceptors = append(a.interceptors, interceptors...)
}

func (a *app) Transport(fn func(any)) {
	a.transportHooks = append(a.transportHooks, fn)
}

func (a *app) Run(opts boot.Options) error {
	internalConfig := bootstrap.Config{
		Address:                opts.Address,
		Constructors:           a.constructors,
		Routes:                 a.routes,
		Interceptors:           a.interceptors,
		TransportHooks:         a.transportHooks,
		EnableGracefulShutdown: opts.EnableGracefulShutdown,
		ShutdownTimeout:        opts.ShutdownTimeout,
	}

	return bootstrap.Run(internalConfig)
}
