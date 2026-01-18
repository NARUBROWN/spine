package bootstrap

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	EnableGracefulShutdown bool
	ShutdownTimeout        time.Duration
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

	// EnableGracefulShutdown 기본값 : false : 즉시 종료 로직
	if !config.EnableGracefulShutdown {
		log.Printf("[Bootstrap] 서버 리스닝 시작: %s", config.Address)
		return server.Start()
	}

	go func() {
		log.Printf("[Bootstrap] 서버 리스닝 시작: %s", config.Address)

		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Bootstrap] 서버 시작 실패: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Bootstrap] 시스템 종료 감지. Graceful Shutdown 시작...")

	timeout := config.ShutdownTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	// 컨텍스트 생성...10초까지 봐줄 것
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("[Bootstrap] 서버 강제 종료 발생: %v", err)
	}

	log.Println("[Bootstrap] 시스템이 안전하게 종료되었습니다.")
	return nil
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
