package bootstrap

import (
	"fmt"
	"log"

	httpEngine "github.com/NARUBROWN/spine/internal/adapter/echo"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/handler"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/NARUBROWN/spine/internal/resolver"
	spineRouter "github.com/NARUBROWN/spine/internal/router"
)

type Config struct {
	Address      string
	Constructors []any
	Routes       []spineRouter.RouteSpec
}

func Run(config Config) error {
	printBanner()

	log.Println("[Bootstrap] 컨테이너 초기화 시작")
	// 컨테이너 생성
	container := container.New()

	log.Printf("[Bootstrap] 생성자 등록 시작 (%d개)", len(config.Constructors))
	// 생성자 등록
	for _, constructor := range config.Constructors {
		log.Printf("[Bootstrap] 생성자 등록 : %T", constructor)
		if err := container.RegisterConstructor(constructor); err != nil {
			return err
		}
	}

	log.Printf("[Bootstrap] 라우터 구성 시작 (%d개 라우트)", len(config.Routes))
	// Router 생성 및 라우트 등록
	router := spineRouter.NewRouter()
	for _, route := range config.Routes {
		log.Printf("[Bootstrap] 라우터 등록 : (%s) %s", route.Method, route.Path)
		meta, err := spineRouter.NewHandlerMeta(route.Handler)
		if err != nil {
			return err
		}
		router.Register(route.Method, route.Path, meta)
	}

	log.Println("[Bootstrap] 컨트롤러 의존성 Warm-up 시작")
	// Warm-Up Component
	if err := container.WarmUp(router.ControllerTypes()); err != nil {
		// Warm-up 실패시 panic
		panic(err)
	}

	log.Println("[Bootstrap] 실행 파이프라인 구성")
	invoker := invoker.NewInvoker(container)
	pipeline := pipeline.NewPipeline(router, invoker)

	pipeline.AddArgumentResolver(
		// Context 리졸버
		&resolver.ContextResolver{},

		// Path 리졸버들
		&resolver.PathIntResolver{},
		&resolver.PathStringResolver{},
		&resolver.PathBooleanResolver{},

		// Query 의미 타입 리졸버들
		&resolver.PaginationResolver{},
		&resolver.QueryValuesResolver{},

		// Body 리졸버
		&resolver.DTOResolver{},
	)

	pipeline.AddReturnValueHandler(
		&handler.StringReturnHandler{},
		&handler.JSONReturnHandler{},
		&handler.ErrorReturnHandler{},
	)

	log.Println("[Bootstrap] HTTP 어댑터 마운트")
	// Echo Adapter
	server := httpEngine.NewServer(pipeline, config.Address)
	server.Mount()

	log.Printf("[Bootstrap] 서버 리스닝 시작: %s", config.Address)
	// Listen
	return server.Start()
}

const spineBanner = `
________       _____             
__  ___/__________(_)___________ 
_____ \___  __ \_  /__  __ \  _ \
____/ /__  /_/ /  / _  / / /  __/
/____/ _  .___//_/  /_/ /_/\___/ 
       /_/        
`

func printBanner() {
	fmt.Print(spineBanner)
	log.Printf("[Bootstrap] Spine version: %s", "v0.1.4")
}
