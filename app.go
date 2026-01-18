package spine

import (
	"time"

	"github.com/NARUBROWN/spine/internal/bootstrap"
	"github.com/NARUBROWN/spine/internal/router"
)

type BootOptions struct {
	Address                string
	EnableGracefulShutdown bool
	ShutdownTimeout        time.Duration
}

type App interface {
	// 생성자 선언
	Constructor(constructors ...any)
	// 라우트 선언
	Route(method string, path string, handler any)
	// 실행
	Run(opts BootOptions) error
}

type app struct {
	constructors []any
	routes       []router.RouteSpec
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

func (a *app) Run(opts BootOptions) error {
	internalConfig := bootstrap.Config{
		Address:                opts.Address,
		Constructors:           a.constructors,
		Routes:                 a.routes,
		EnableGracefulShutdown: opts.EnableGracefulShutdown,
		ShutdownTimeout:        opts.ShutdownTimeout,
	}

	return bootstrap.Run(internalConfig)
}
