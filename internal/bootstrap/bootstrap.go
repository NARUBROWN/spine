package bootstrap

import (
	"reflect"

	httpEngine "github.com/NARUBROWN/spine/internal/adapter/echo"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/handler"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/NARUBROWN/spine/internal/resolver"
	spineRouter "github.com/NARUBROWN/spine/internal/router"
	"github.com/labstack/echo/v4"
)

type Config struct {
	Address        string
	ComponentTypes []reflect.Type
	Constructors   []any
	Routes         []spineRouter.RouteSpec
}

func Run(config Config) error {
	// 컨테이너 생성
	container := container.New()

	// 컴포넌트 타입 등록
	for _, componentType := range config.ComponentTypes {
		container.RegisterComponent(componentType)
	}

	// 생성자 등록
	for _, constructor := range config.Constructors {
		if err := container.RegisterConstructor(constructor); err != nil {
			return err
		}
	}

	// Router 생성 및 라우트 등록
	router := spineRouter.NewRouter()
	for _, route := range config.Routes {
		meta, err := spineRouter.NewHandlerMeta(route.Handler)
		if err != nil {
			return err
		}
		router.Register(route.Method, route.Path, meta)
	}

	// Resolver Registry
	argRegistry := resolver.NewRegistry(
		&resolver.ContextResolver{},
		&resolver.QueryDTOResolver{},
		&resolver.DTOResolver{},
		&resolver.PrimitiveResolver{},
	)

	returnRegistry := handler.NewReturnHandlerRegistry(
		&handler.StringReturnHandler{},
		&handler.JSONReturnHandler{},
		&handler.ErrorReturnHandler{},
	)

	invoke := invoker.NewInvoker(container, argRegistry, returnRegistry)
	pipe := pipeline.NewPipeline(invoke)

	// Echo Adapter
	echo := echo.New()
	adapter := httpEngine.NewAdapter(router, pipe)
	adapter.Mount(echo)

	// Listen
	return echo.Start(config.Address)
}
