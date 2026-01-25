package bootstrap

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"syscall"
	"time"

	"github.com/NARUBROWN/spine/core"
	httpEngine "github.com/NARUBROWN/spine/internal/adapter/echo"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/event/consumer"
	eventResolver "github.com/NARUBROWN/spine/internal/event/consumer/resolver"
	"github.com/NARUBROWN/spine/internal/event/extract"
	"github.com/NARUBROWN/spine/internal/event/hook"
	"github.com/NARUBROWN/spine/internal/event/infra/kafka"
	"github.com/NARUBROWN/spine/internal/event/infra/rabbitmq"
	"github.com/NARUBROWN/spine/internal/event/publish"
	"github.com/NARUBROWN/spine/internal/handler"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/NARUBROWN/spine/internal/resolver"
	spineRouter "github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type Config struct {
	Address                string
	Constructors           []any
	Routes                 []spineRouter.RouteSpec
	Interceptors           []core.Interceptor
	TransportHooks         []func(any)
	EnableGracefulShutdown bool
	ShutdownTimeout        time.Duration
	Kafka                  *boot.KafkaOptions
	RabbitMQ               *boot.RabbitMqOptions
	ConsumerRegistry       *consumer.Registry
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
		meta.Interceptors = route.Interceptors
		router.Register(route.Method, route.Path, meta)
	}

	log.Println("[Bootstrap] 컨트롤러 의존성 Warm-up 시작")
	// Warm-Up Component
	if err := container.WarmUp(router.ControllerTypes()); err != nil {
		// Warm-up 실패시 panic
		panic(err)
	}

	log.Println("[Bootstrap] 실행 파이프라인 구성")
	httpInvoker := invoker.NewInvoker(container)
	httpPipeline := pipeline.NewPipeline(router, httpInvoker)

	log.Println("[Bootstrap] ArgumentResolver 등록")
	httpPipeline.AddArgumentResolver(
		// 표준 Context 리졸버
		&resolver.StdContextResolver{},

		// Header Resolver
		&resolver.HeaderResolver{},

		// Path 리졸버들
		&resolver.PathIntResolver{},
		&resolver.PathStringResolver{},
		&resolver.PathBooleanResolver{},

		// Query 의미 타입 리졸버들
		&resolver.PaginationResolver{},
		&resolver.QueryValuesResolver{},

		// Body 리졸버
		&resolver.DTOResolver{},

		// Form DTO (multipart / form)
		&resolver.FormDTOResolver{},

		// Multipart files
		&resolver.UploadedFilesResolver{},
	)

	log.Println("[Bootstrap] ReturnValueHandler 등록")
	httpPipeline.AddReturnValueHandler(
		&handler.StringReturnHandler{},
		&handler.JSONReturnHandler{},
		&handler.ErrorReturnHandler{},
	)

	log.Println("[Bootstrap] Interceptor 등록 시작")

	// 라우트 레벨 인터셉터 (파이프라인에 등록하지 않음)
	for i, route := range config.Routes {
		resolved := make([]core.Interceptor, len(route.Interceptors))
		for i, interceptor := range route.Interceptors {
			interceptorType := reflect.TypeOf(interceptor)
			value := reflect.ValueOf(interceptor)

			if interceptorType.Kind() == reflect.Pointer && value.IsNil() {
				log.Printf("[Bootstrap] Route Interceptor %s가 컨테이너에서 생성됐습니다.", interceptorType.Elem().Name())
				inst, err := container.Resolve(interceptorType)
				if err != nil {
					panic(err)
				}
				resolved[i] = inst.(core.Interceptor)
			} else {
				log.Printf("[Bootstrap] Route Interceptor %T가 인스턴스에서 사용됩니다.", interceptor)
				resolved[i] = interceptor
			}
		}
		config.Routes[i].Interceptors = resolved
	}

	log.Println("[Bootstrap] 라우트 레벨 Interceptor resolve 완료")

	// Kafka Write 옵션이 존재하면 Write를 Boot에 포함
	if config.Kafka != nil && config.Kafka.Write != nil {
		log.Println("[Bootstrap] Event Queue (Kafka) 구성")

		kafkaPublisher := kafka.NewKafkaPublisher(&boot.KafkaOptions{
			Brokers: config.Kafka.Brokers,
			Write: &boot.KafkaWriteOptions{
				TopicPrefix: config.Kafka.Write.TopicPrefix,
			},
		})

		dispatcher := &publish.DefaultEventDispatcher{
			Publishers: []publish.EventPublisher{
				kafkaPublisher,
			},
		}

		eventHook := &hook.EventDispatchHook{
			Extractor:  extract.DefaultEventExtractor{},
			Dispatcher: dispatcher,
		}

		httpPipeline.AddPostExecutionHook(eventHook)
	}

	// Kafka Read 옵션이 존재하면 Read를 Boot에 포함
	if config.Kafka != nil && config.Kafka.Read != nil && config.ConsumerRegistry != nil && len(config.ConsumerRegistry.Registrations()) > 0 {
		log.Println("[Bootstrap] 이벤트 컨슈머 런타임을 초기화합니다.")

		factory := kafka.NewRunnerFactory(boot.KafkaOptions{
			Brokers: config.Kafka.Brokers,
			Read: &boot.KafkaReadOptions{
				GroupID: config.Kafka.Read.GroupID,
			},
		})

		consumerRouter := spineRouter.NewRouter()
		for _, registration := range config.ConsumerRegistry.Registrations() {
			log.Printf("[Bootstrap] 컨슈머 라우터 등록 : (EVENT) %s", registration.Topic)
			consumerRouter.Register("EVENT", registration.Topic, registration.Meta)
		}

		consumerInvoker := invoker.NewInvoker(container)
		consumerPipeline := pipeline.NewPipeline(consumerRouter, consumerInvoker)

		consumerPipeline.AddArgumentResolver(
			// 표준 context.Context
			&resolver.StdContextResolver{},

			// Consumer 전용 리졸버
			&eventResolver.EventNameResolver{},
			&eventResolver.EventBusResolver{},
			&eventResolver.PayloadResolver{},
			&eventResolver.DTOResolver{},
		)

		consumerPipeline.AddReturnValueHandler(
			&handler.ErrorReturnHandler{},
			&handler.StringReturnHandler{},
			&handler.JSONReturnHandler{},
		)

		runtime := consumer.NewRuntime(
			config.ConsumerRegistry,
			factory,
			consumerPipeline,
		)

		go runtime.Start(context.Background())
		defer runtime.Stop()
	}

	// RabbitMQ 쓰기 설정이 존재하면, 발행 구성
	if config.RabbitMQ != nil && config.RabbitMQ.Write != nil {
		log.Println("[Bootstrap] RabbitMQ 이벤트 발행 구성")

		rabbitmqWriter := rabbitmq.NewRabbitMqWriter(boot.RabbitMqOptions{
			URL: config.RabbitMQ.URL,
			Write: &boot.RabbitMqWriteOptions{
				Exchange:   config.RabbitMQ.Write.Exchange,
				RoutingKey: config.RabbitMQ.Write.RoutingKey,
			},
		})

		dispatcher := &publish.DefaultEventDispatcher{
			Publishers: []publish.EventPublisher{
				rabbitmqWriter,
			},
		}

		eventHook := &hook.EventDispatchHook{
			Extractor:  extract.DefaultEventExtractor{},
			Dispatcher: dispatcher,
		}

		httpPipeline.AddPostExecutionHook(eventHook)
	}

	// RabbitMQ 읽기 설정이 존재하면, 컨슈머 구성
	if config.RabbitMQ != nil && config.RabbitMQ.Read != nil && config.ConsumerRegistry != nil && len(config.ConsumerRegistry.Registrations()) > 0 {
		log.Println("[Bootstrap] RabbitMQ 이벤트 컨슈머 구성")

		factory := rabbitmq.NewRunnerFactory(boot.RabbitMqOptions{
			URL: config.RabbitMQ.URL,
			Read: &boot.RabbitMqReadOptions{
				Queue:      config.RabbitMQ.Read.Queue,
				Exchange:   config.RabbitMQ.Read.Exchange,
				RoutingKey: config.RabbitMQ.Read.RoutingKey,
			},
		})

		consumerRouter := spineRouter.NewRouter()
		for _, registration := range config.ConsumerRegistry.Registrations() {
			log.Printf("[Bootstrap] 컨슈머 라우터 등록 : (EVENT) %s", registration.Topic)
			consumerRouter.Register("EVENT", registration.Topic, registration.Meta)
		}

		consumerInvoker := invoker.NewInvoker(container)
		consumerPipeline := pipeline.NewPipeline(consumerRouter, consumerInvoker)

		consumerPipeline.AddArgumentResolver(
			// 표준 context.Context
			&resolver.StdContextResolver{},

			// Consumer 전용 리졸버
			&eventResolver.EventNameResolver{},
			&eventResolver.EventBusResolver{},
			&eventResolver.PayloadResolver{},
			&eventResolver.DTOResolver{},
		)

		consumerPipeline.AddReturnValueHandler(
			&handler.ErrorReturnHandler{},
			&handler.StringReturnHandler{},
			&handler.JSONReturnHandler{},
		)

		runtime := consumer.NewRuntime(
			config.ConsumerRegistry,
			factory,
			consumerPipeline,
		)

		go runtime.Start(context.Background())
		defer runtime.Stop()
	}

	uniqueInterceptors := make(map[reflect.Type]core.Interceptor)

	// 전역 인터셉터 수집
	for _, interceptor := range config.Interceptors {
		t := reflect.TypeOf(interceptor)
		uniqueInterceptors[t] = interceptor
	}

	for _, interceptor := range uniqueInterceptors {
		v := reflect.ValueOf(interceptor)
		t := reflect.TypeOf(interceptor)

		if t.Kind() == reflect.Pointer && v.IsNil() {
			log.Printf("[Bootstrap] Interceptor %s가 컨테이너에서 생성됐습니다.", t.Elem().Name())

			inst, err := container.Resolve(t)
			if err != nil {
				panic(err)
			}

			httpPipeline.AddInterceptor(inst.(core.Interceptor))
			continue
		}

		log.Printf("[Bootstrap] Interceptor %T가 인스턴스에서 사용됩니다.", interceptor)
		httpPipeline.AddInterceptor(interceptor)
	}

	log.Println("[Bootstrap] HTTP 어댑터 마운트")
	// Echo Adapter
	server := httpEngine.NewServer(httpPipeline, config.Address, config.TransportHooks)
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
	log.Printf("[Bootstrap] Spine version: %s", "v0.2.4")
}
