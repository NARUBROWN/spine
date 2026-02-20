package bootstrap

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/NARUBROWN/spine/core"
	httpEngine "github.com/NARUBROWN/spine/internal/adapter/echo"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/event/consumer"
	eventResolver "github.com/NARUBROWN/spine/internal/event/consumer/resolver"
	"github.com/NARUBROWN/spine/internal/event/hook"
	"github.com/NARUBROWN/spine/internal/event/infra/kafka"
	"github.com/NARUBROWN/spine/internal/event/infra/rabbitmq"
	eventPublish "github.com/NARUBROWN/spine/internal/event/publish"
	"github.com/NARUBROWN/spine/internal/handler"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/NARUBROWN/spine/internal/resolver"
	spineRouter "github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/internal/ws"
	wsResolver "github.com/NARUBROWN/spine/internal/ws/resolver"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/labstack/echo/v4"
)

type Config struct {
	Address                string
	Constructors           []any
	Routes                 []spineRouter.RouteSpec
	Interceptors           []core.Interceptor
	TransportHooks         []func(any)
	CustomTransports       []core.CustomTransport
	EnableGracefulShutdown bool
	ShutdownTimeout        time.Duration
	Kafka                  *boot.KafkaOptions
	RabbitMQ               *boot.RabbitMqOptions
	ConsumerRegistry       *consumer.Registry
	HTTP                   *boot.HTTPOptions
	WebSocketRegistry      *ws.Registry
}

type containerFacade struct {
	container *container.Container
}

func (f *containerFacade) Resolve(t reflect.Type) (any, error) {
	return f.container.Resolve(t)
}

func Run(config Config) error {
	printBanner()

	log.Println("[Bootstrap] 컨테이너 초기화 시작")
	// 컨테이너 생성
	container := container.New()

	log.Printf("[Bootstrap] 생성자 등록 시작 (%d개)", len(config.Constructors))
	// 생성자 등록 (HTTP/Consumer 공통)
	for _, constructor := range config.Constructors {
		log.Printf("[Bootstrap] 생성자 등록 : %T", constructor)
		if err := container.RegisterConstructor(constructor); err != nil {
			return err
		}
	}

	// 이벤트 발행기 모음 (Kafka/RabbitMQ 등 옵션에 따라 채워짐)
	var eventPublishers []eventPublish.EventPublisher

	// Kafka Write 옵션이 존재하면 Publisher 구성
	if config.Kafka != nil && config.Kafka.Write != nil {
		log.Println("[Bootstrap] Kafka 이벤트 발행 구성")

		kafkaPublisher, err := kafka.NewKafkaPublisher(&boot.KafkaOptions{
			Brokers: config.Kafka.Brokers,
			Write: &boot.KafkaWriteOptions{
				TopicPrefix: config.Kafka.Write.TopicPrefix,
			},
		})
		if err != nil {
			panic(err)
		}
		eventPublishers = append(eventPublishers, kafkaPublisher)
		defer func() {
			if err := kafkaPublisher.Close(); err != nil {
				log.Printf("[Bootstrap] Kafka publisher close 실패: %v", err)
			}
		}()
	}

	// RabbitMQ Write 옵션이 존재하면 Publisher 구성
	if config.RabbitMQ != nil && config.RabbitMQ.Write != nil {
		log.Println("[Bootstrap] RabbitMQ 이벤트 발행 구성")

		rabbitmqWriter, err := rabbitmq.NewRabbitMqWriter(boot.RabbitMqOptions{
			URL: config.RabbitMQ.URL,
			Write: &boot.RabbitMqWriteOptions{
				Exchange: config.RabbitMQ.Write.Exchange,
			},
		})
		if err != nil {
			panic(err)
		}
		eventPublishers = append(eventPublishers, rabbitmqWriter)
		defer func() {
			if err := rabbitmqWriter.Close(); err != nil {
				log.Printf("[Bootstrap] RabbitMQ writer close 실패: %v", err)
			}
		}()
	}

	// PostExecutionHook에서 사용할 공통 Dispatcher (Publishers가 없으면 nil 유지)
	var dispatchHook *hook.EventDispatchHook
	if len(eventPublishers) > 0 {
		dispatchHook = &hook.EventDispatchHook{
			Dispatcher: &eventPublish.DefaultEventDispatcher{Publishers: eventPublishers},
		}
	}

	var server *httpEngine.Server
	var httpErrCh chan error
	var consumerErrCh chan error
	var customTransportErrCh chan error
	var wsRuntime *ws.Runtime

	stopCustomTransportOnce := sync.Once{}
	stopCustomTransports := func(ctx context.Context) {
		stopCustomTransportOnce.Do(func() {
			for _, transport := range config.CustomTransports {
				if transport == nil {
					continue
				}
				if err := transport.Stop(ctx); err != nil {
					log.Printf("[Bootstrap] CustomTransport 종료 실패: %v", err)
				}
			}
		})
	}

	if len(config.CustomTransports) > 0 {
		log.Printf("[Bootstrap] CustomTransport 초기화 시작 (%d개)", len(config.CustomTransports))
		facade := &containerFacade{container: container}

		for i, transport := range config.CustomTransports {
			if transport == nil {
				return fmt.Errorf("[Bootstrap] CustomTransport[%d]가 nil입니다", i)
			}
			if err := transport.Init(facade); err != nil {
				return fmt.Errorf("[Bootstrap] CustomTransport Init 실패: %w", err)
			}
		}

		customTransportErrCh = make(chan error, len(config.CustomTransports))
		for _, transport := range config.CustomTransports {
			transport := transport
			go func() {
				if err := transport.Start(); err != nil {
					customTransportErrCh <- err
				}
			}()
		}

		defer func() {
			timeout := config.ShutdownTimeout
			if timeout == 0 {
				timeout = 10 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			stopCustomTransports(ctx)
		}()
	}

	if config.HTTP != nil {
		prefix := config.HTTP.GlobalPrefix
		if prefix != "" {
			if !strings.HasPrefix(prefix, "/") {
				panic("HTTP GlobalPrefix는 '/'로 시작해야 합니다")
			}
			if strings.Contains(prefix, ":") {
				panic("Path 파라미터는 HTTP GlobalPrefix에서 사용될 수 없습니다")
			}
			if strings.Contains(prefix, "*") {
				panic("와일드카드는 HTTP GlobalPrefix에서 사용될 수 없습니다")
			}
			prefix = strings.TrimSuffix(prefix, "/")
			log.Printf("[Bootstrap] HTTP GlobalPrefix 적용: %s", prefix)
		}

		log.Printf("[Bootstrap] 라우터 구성 시작 (%d개 라우트)", len(config.Routes))
		// Router 생성 및 라우트 등록
		router := spineRouter.NewRouter()

		registeredPathsByMethod := make(map[string][]string)

		loggedRouteInterceptors := make(map[reflect.Type]bool)

		for _, route := range config.Routes {
			log.Printf("[Bootstrap] 라우터 등록 : (%s) %s", route.Method, route.Path)
			meta, err := spineRouter.NewHandlerMeta(route.Handler)
			if err != nil {
				return err
			}

			resolved := make([]core.Interceptor, len(route.Interceptors))
			for i, interceptor := range route.Interceptors {
				interceptorType := reflect.TypeOf(interceptor)
				value := reflect.ValueOf(interceptor)

				// 같은 타입의 인터셉터 로깅은 한 번만 남긴다.
				logged := loggedRouteInterceptors[interceptorType]

				if interceptorType.Kind() == reflect.Pointer && value.IsNil() {
					if !logged {
						log.Printf("[Bootstrap] Route Interceptor %s가 컨테이너에서 생성됐습니다.", interceptorType.Elem().Name())
						loggedRouteInterceptors[interceptorType] = true
					}

					inst, err := container.Resolve(interceptorType)
					if err != nil {
						panic(err)
					}
					resolved[i] = inst.(core.Interceptor)
				} else {
					if !logged {
						log.Printf("[Bootstrap] Route Interceptor %T가 인스턴스에서 사용됩니다.", interceptor)
						loggedRouteInterceptors[interceptorType] = true
					}
					resolved[i] = interceptor
				}
			}

			meta.Interceptors = resolved
			fullPath := joinPath(prefix, route.Path)

			assertNoAmbiguousRoute(route.Method, fullPath, registeredPathsByMethod[route.Method])
			registeredPathsByMethod[route.Method] = append(registeredPathsByMethod[route.Method], fullPath)

			router.Register(route.Method, fullPath, meta)
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

		// HTTP PostExecutionHook: 도메인 이벤트 발행 (퍼블리셔가 있는 경우에만)
		if dispatchHook != nil {
			httpPipeline.AddPostExecutionHook(dispatchHook)
		}

		log.Println("[Bootstrap] ArgumentResolver 등록")
		httpPipeline.AddArgumentResolver(
			// 표준 Context 리졸버
			&resolver.StdContextResolver{},

			// Spine Controller Context View
			&resolver.ControllerContextResolver{},

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
			&handler.RedirectReturnValueHandler{},
			&handler.BinaryReturnHandler{},
			&handler.StringReturnHandler{},
			&handler.JSONReturnHandler{},
			&handler.ErrorReturnHandler{},
		)

		log.Println("[Bootstrap] Interceptor 등록 시작")

		// 전역 인터셉터 수집 (중복 타입은 최초 등록 순서를 유지)
		seen := make(map[reflect.Type]struct{})
		ordered := make([]core.Interceptor, 0, len(config.Interceptors))
		for _, interceptor := range config.Interceptors {
			t := reflect.TypeOf(interceptor)
			if _, ok := seen[t]; ok {
				continue
			}
			seen[t] = struct{}{}
			ordered = append(ordered, interceptor)
		}

		for _, interceptor := range ordered {
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

		// WebSocket Runtime 구성
		if config.WebSocketRegistry != nil && len(config.WebSocketRegistry.Registrations()) > 0 {
			log.Println("[Bootstrap] WebSocket 런타임 구성")

			// WS 전용 ArgumentResolver 등록
			wsPipeline := buildWSPipeline(container, config.WebSocketRegistry, dispatchHook)

			wsRuntime = ws.NewRuntime(config.WebSocketRegistry, wsPipeline)
			defer wsRuntime.Stop()

			// Echo Transport Hook으로 마운트
			config.TransportHooks = append(config.TransportHooks, func(e any) {
				echoInstance, ok := e.(*echo.Echo)
				if !ok {
					return
				}
				for _, reg := range config.WebSocketRegistry.Registrations() {
					echoInstance.GET(reg.Path, func(c echo.Context) error {
						wsRuntime.HandleConn(c.Response().Writer, c.Request(), reg)
						return nil
					})
				}
			})
		}

		// Echo Adapter
		recoverEnabled := config.HTTP == nil || !config.HTTP.DisableRecover
		server = httpEngine.NewServer(httpPipeline, config.Address, config.TransportHooks, recoverEnabled)
		server.Mount()

		log.Printf("[Bootstrap] 서버 리스닝 시작: %s", config.Address)
		httpErrCh = make(chan error, 1)
		go func() {
			if err := server.Start(); err != nil && err != http.ErrServerClosed {
				httpErrCh <- err
			}
		}()
	}

	// Consumer 컨트롤러 Warm-up
	if config.ConsumerRegistry != nil {
		log.Println("[Bootstrap] Consumer 컨트롤러 의존성 Warm-up 시작")
		var consumerTypes []reflect.Type
		for _, reg := range config.ConsumerRegistry.Registrations() {
			consumerTypes = append(consumerTypes, reg.Meta.ControllerType)
		}
		if err := container.WarmUp(consumerTypes); err != nil {
			panic(err)
		}
	}

	// Kafka Read 옵션이 존재하면 Read를 Boot에 포함
	consumerStarted := false

	if config.Kafka != nil && config.Kafka.Read != nil && config.ConsumerRegistry != nil && len(config.ConsumerRegistry.Registrations()) > 0 {
		log.Println("[Bootstrap] Kafka 이벤트 컨슈머 구성")
		if consumerErrCh == nil {
			consumerErrCh = make(chan error, 1)
		}

		factory := kafka.NewRunnerFactory(boot.KafkaOptions{
			Brokers: config.Kafka.Brokers,
			Read: &boot.KafkaReadOptions{
				GroupID: config.Kafka.Read.GroupID,
			},
		})

		consumerPipeline := buildConsumerPipeline(container, config.ConsumerRegistry, dispatchHook)

		runtime := consumer.NewRuntime(
			config.ConsumerRegistry,
			factory,
			consumerPipeline,
		)

		if err := runtime.Validate(); err != nil {
			panic(err)
		}

		forwardConsumerErrors("Kafka", runtime, consumerErrCh)
		go runtime.Start(context.Background())
		defer runtime.Stop()
		consumerStarted = true
	}

	// RabbitMQ 읽기 설정이 존재하면, 컨슈머 구성
	if config.RabbitMQ != nil && config.RabbitMQ.Read != nil && config.ConsumerRegistry != nil && len(config.ConsumerRegistry.Registrations()) > 0 {
		log.Println("[Bootstrap] RabbitMQ 이벤트 컨슈머 구성")
		if consumerErrCh == nil {
			consumerErrCh = make(chan error, 1)
		}

		factory := rabbitmq.NewRunnerFactory(boot.RabbitMqOptions{
			URL: config.RabbitMQ.URL,
			Read: &boot.RabbitMqReadOptions{
				Exchange: config.RabbitMQ.Read.Exchange,
			},
		})

		consumerPipeline := buildConsumerPipeline(container, config.ConsumerRegistry, dispatchHook)

		runtime := consumer.NewRuntime(
			config.ConsumerRegistry,
			factory,
			consumerPipeline,
		)

		if err := runtime.Validate(); err != nil {
			panic(err)
		}

		forwardConsumerErrors("RabbitMQ", runtime, consumerErrCh)
		go runtime.Start(context.Background())
		defer runtime.Stop()
		consumerStarted = true
	}

	if config.HTTP != nil {
		// Graceful 비활성화: 서버가 종료될 때까지 블록
		if !config.EnableGracefulShutdown {
			select {
			case err := <-httpErrCh:
				if err != nil {
					return err
				}
				return nil
			case err := <-consumerErrCh:
				return err
			case err := <-customTransportErrCh:
				return err
			}
		}

		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

		select {
		case err := <-httpErrCh:
			if err != nil {
				return err
			}
		case err := <-consumerErrCh:
			return err
		case err := <-customTransportErrCh:
			return err
		case <-quit:
		}

		log.Println("[Bootstrap] 시스템 종료 감지. Graceful Shutdown 시작...")

		if wsRuntime != nil {
			wsRuntime.Stop()
		}

		timeout := config.ShutdownTimeout
		if timeout == 0 {
			timeout = 10 * time.Second
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		stopCustomTransports(ctx)

		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("[Bootstrap] 서버 강제 종료 발생: %v", err)
		}

		log.Println("[Bootstrap] 시스템이 안전하게 종료되었습니다.")
	}

	// HTTP가 비활성화된 상태에서 이벤트 컨슈머만 실행 중이면 종료 신호를 기다린다.
	if config.HTTP == nil && (consumerStarted || customTransportErrCh != nil) {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-quit:
			log.Println("[Bootstrap] 시스템 종료 감지. 런타임 중지...")
		case err := <-consumerErrCh:
			return err
		case err := <-customTransportErrCh:
			return err
		}

		timeout := config.ShutdownTimeout
		if timeout == 0 {
			timeout = 10 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		stopCustomTransports(ctx)
	}

	return nil
}

func joinPath(prefix, path string) string {
	if path == "" {
		panic("라우트 Path는 비어있을 수 없습니다")
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	if prefix == "" {
		return path
	}

	return prefix + path
}

func assertNoAmbiguousRoute(method, newPath string, existing []string) {
	newSegs := splitPathForValidation(newPath)

	for _, oldPath := range existing {
		oldSegs := splitPathForValidation(oldPath)

		// 서로 다른 segment length는 절대 겹치지 않음
		if len(newSegs) != len(oldSegs) {
			continue
		}

		// 각 segment가 충돌 없이 겹치는지(교집합 존재) 검사
		overlaps := true
		for i := 0; i < len(newSegs); i++ {
			a := newSegs[i]
			b := oldSegs[i]

			aParam := isPathParam(a)
			bParam := isPathParam(b)

			// 둘 다 literal인데 값이 다르면 이 위치에서 교집합이 사라짐
			if !aParam && !bParam && a != b {
				overlaps = false
				break
			}
		}

		if overlaps {
			panic(fmt.Sprintf(
				"[Router] 모호한 라우트가 감지되었습니다 (부트 타임): method=%s, 신규=%s 가 기존=%s 와 충돌합니다",
				method, newPath, oldPath,
			))
		}
	}
}

func splitPathForValidation(path string) []string {
	p := strings.TrimSpace(path)
	if p == "" || p == "/" {
		return []string{}
	}

	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return []string{}
	}

	return strings.Split(p, "/")
}

func isPathParam(seg string) bool {
	return strings.HasPrefix(seg, ":")
}

// forwardConsumerErrors는 특정 런타임의 치명적 에러를 공용 채널로 전달한다.
func forwardConsumerErrors(name string, runtime *consumer.Runtime, out chan<- error) {
	go func() {
		if err := <-runtime.Errors(); err != nil {
			wrapped := fmt.Errorf("[Bootstrap] %s consumer runtime error: %w", name, err)
			select {
			case out <- wrapped:
			default:
				log.Printf("%v (consumerErrCh가 가득 차 전파하지 못했습니다)", wrapped)
			}
		}
	}()
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
	log.Printf("[Bootstrap] Spine version: %s", "v0.4.1")
}

func buildConsumerPipeline(container *container.Container, registry *consumer.Registry, dispatchHook *hook.EventDispatchHook) *pipeline.Pipeline {
	consumerRouter := spineRouter.NewRouter()
	for _, registration := range registry.Registrations() {
		consumerRouter.Register("EVENT", registration.Topic, registration.Meta)
	}

	consumerInvoker := invoker.NewInvoker(container)
	consumerPipeline := pipeline.NewPipeline(consumerRouter, consumerInvoker)

	if dispatchHook != nil {
		consumerPipeline.AddPostExecutionHook(dispatchHook)
	}

	consumerPipeline.AddArgumentResolver(
		&resolver.StdContextResolver{},
		&eventResolver.EventNameResolver{},
		&eventResolver.PayloadResolver{},
		&eventResolver.DTOResolver{},
	)

	return consumerPipeline
}

func buildWSPipeline(
	container *container.Container,
	registry *ws.Registry,
	dispatchHook *hook.EventDispatchHook,
) *pipeline.Pipeline {
	wsRouter := spineRouter.NewRouter()
	for _, reg := range registry.Registrations() {
		wsRouter.Register("WS", reg.Path, reg.Meta)
	}

	wsInvoker := invoker.NewInvoker(container)
	wsPipeline := pipeline.NewPipeline(wsRouter, wsInvoker)

	if dispatchHook != nil {
		wsPipeline.AddPostExecutionHook(dispatchHook)
	}

	wsPipeline.AddArgumentResolver(
		&resolver.StdContextResolver{},
		&wsResolver.ConnectionIDResolver{},
		&wsResolver.PayloadResolver{},
		&wsResolver.DTOResolver{},
	)

	return wsPipeline
}
