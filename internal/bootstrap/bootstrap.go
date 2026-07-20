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

	// Graceful shutdown signals must be subscribed before the HTTP handler is
	// exposed through transport hooks. Otherwise a shutdown arriving during
	// startup can still terminate the process with the operating-system default.
	var httpShutdownSignals chan os.Signal
	if config.HTTP != nil && config.EnableGracefulShutdown {
		httpShutdownSignals = make(chan os.Signal, 1)
		signal.Notify(httpShutdownSignals, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(httpShutdownSignals)
	}

	log.Println("[Bootstrap] Initializing container")
	// 컨테이너 생성
	container := container.New()

	log.Printf("[Bootstrap] Registering constructors (%d)", len(config.Constructors))
	// 생성자 등록 (HTTP/Consumer 공통)
	for _, constructor := range config.Constructors {
		log.Printf("[Bootstrap] Registering constructor: %T", constructor)
		if err := container.RegisterConstructor(constructor); err != nil {
			return err
		}
	}

	// 이벤트 발행기 모음 (Kafka/RabbitMQ 등 옵션에 따라 채워짐)
	var eventPublishers []eventPublish.EventPublisher

	// Kafka Write 옵션이 존재하면 Publisher 구성
	if config.Kafka != nil && config.Kafka.Write != nil {
		log.Println("[Bootstrap] Configuring Kafka publisher")

		kafkaPublisher, err := kafka.NewKafkaPublisher(&boot.KafkaOptions{
			Brokers: config.Kafka.Brokers,
			Write: &boot.KafkaWriteOptions{
				TopicPrefix: config.Kafka.Write.TopicPrefix,
			},
		})
		if err != nil {
			return fmt.Errorf("[Bootstrap] failed to initialize Kafka publisher: %w", err)
		}
		eventPublishers = append(eventPublishers, kafkaPublisher)
		defer func() {
			if err := kafkaPublisher.Close(); err != nil {
				log.Printf("[Bootstrap] failed to close Kafka publisher: %v", err)
			}
		}()
	}

	// RabbitMQ Write 옵션이 존재하면 Publisher 구성
	if config.RabbitMQ != nil && config.RabbitMQ.Write != nil {
		log.Println("[Bootstrap] Configuring RabbitMQ publisher")

		rabbitmqWriter, err := rabbitmq.NewRabbitMqWriter(boot.RabbitMqOptions{
			URL: config.RabbitMQ.URL,
			Write: &boot.RabbitMqWriteOptions{
				Exchange: config.RabbitMQ.Write.Exchange,
			},
		})
		if err != nil {
			return fmt.Errorf("[Bootstrap] failed to initialize RabbitMQ writer: %w", err)
		}
		eventPublishers = append(eventPublishers, rabbitmqWriter)
		defer func() {
			if err := rabbitmqWriter.Close(); err != nil {
				log.Printf("[Bootstrap] failed to close RabbitMQ writer: %v", err)
			}
		}()
	}

	// PostExecutionHook에서 사용할 공통 Dispatcher (Publishers가 없으면 nil 유지)
	var dispatchHook *hook.EventDispatchHook
	if len(eventPublishers) > 0 {
		dispatcher, err := eventPublish.NewDefaultEventDispatcher(eventPublishers...)
		if err != nil {
			return fmt.Errorf("[Bootstrap] failed to initialize event dispatcher: %w", err)
		}
		dispatchHook = &hook.EventDispatchHook{
			Dispatcher: dispatcher,
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
					log.Printf("[Bootstrap] failed to stop custom transport: %v", err)
				}
			}
		})
	}

	if len(config.CustomTransports) > 0 {
		log.Printf("[Bootstrap] Initializing custom transports (%d)", len(config.CustomTransports))
		facade := &containerFacade{container: container}

		for i, transport := range config.CustomTransports {
			if transport == nil {
				return fmt.Errorf("[Bootstrap] custom transport[%d] is nil", i)
			}
			if err := transport.Init(facade); err != nil {
				return fmt.Errorf("[Bootstrap] custom transport initialization failed: %w", err)
			}
		}

		customTransportErrCh = make(chan error, len(config.CustomTransports))
		for _, transport := range config.CustomTransports {
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
				return fmt.Errorf("HTTP global prefix must start with '/'")
			}
			if strings.Contains(prefix, ":") {
				return fmt.Errorf("path parameters are not allowed in the HTTP global prefix")
			}
			if strings.Contains(prefix, "*") {
				return fmt.Errorf("wildcards are not allowed in the HTTP global prefix")
			}
			prefix = strings.TrimSuffix(prefix, "/")
			log.Printf("[Bootstrap] Applied HTTP global prefix: %s", prefix)
		}

		log.Printf("[Bootstrap] Configuring HTTP routes (%d routes)", len(config.Routes))
		// Router 생성 및 라우트 등록
		router := spineRouter.NewRouter()

		registeredPathsByMethod := make(map[string][]string)

		loggedRouteInterceptors := make(map[reflect.Type]bool)

		for _, route := range config.Routes {
			meta, err := spineRouter.NewHandlerMeta(route.Handler)
			if err != nil {
				return err
			}

			resolved := make([]core.Interceptor, len(route.Interceptors))
			for i, interceptor := range route.Interceptors {
				interceptorType := reflect.TypeOf(interceptor)
				if interceptorType == nil {
					return fmt.Errorf("[Bootstrap] route interceptor[%d] is nil", i)
				}
				value := reflect.ValueOf(interceptor)

				// 같은 타입의 인터셉터 로깅은 한 번만 남긴다.
				logged := loggedRouteInterceptors[interceptorType]

				if interceptorType.Kind() == reflect.Pointer && value.IsNil() {
					if !logged {
						log.Printf("[Bootstrap] Created route interceptor %s from the container", interceptorType.Elem().Name())
						loggedRouteInterceptors[interceptorType] = true
					}

					inst, err := container.Resolve(interceptorType)
					if err != nil {
						return fmt.Errorf("[Bootstrap] failed to create route interceptor: %w", err)
					}
					resolved[i] = inst.(core.Interceptor)
				} else {
					if !logged {
						log.Printf("[Bootstrap] Using route interceptor instance: %T", interceptor)
						loggedRouteInterceptors[interceptorType] = true
					}
					resolved[i] = interceptor
				}
			}

			meta.Interceptors = resolved
			fullPath, err := joinPath(prefix, route.Path)
			if err != nil {
				return err
			}
			log.Printf("[Bootstrap] Registered HTTP route: (%s) %s", route.Method, fullPath)

			if err := assertNoAmbiguousRoute(route.Method, fullPath, registeredPathsByMethod[route.Method]); err != nil {
				return err
			}
			registeredPathsByMethod[route.Method] = append(registeredPathsByMethod[route.Method], fullPath)

			router.Register(route.Method, fullPath, meta)
		}

		log.Println("[Bootstrap] Warming up controller dependencies")
		// Warm-Up Component
		if err := container.WarmUp(router.ControllerTypes()); err != nil {
			return fmt.Errorf("[Bootstrap] HTTP controller warm-up failed: %w", err)
		}

		log.Println("[Bootstrap] Building execution pipeline")
		httpInvoker := invoker.NewInvoker(container)
		httpPipeline := pipeline.NewPipeline(router, httpInvoker)

		// HTTP PostExecutionHook: 도메인 이벤트 발행 (퍼블리셔가 있는 경우에만)
		if dispatchHook != nil {
			httpPipeline.AddPostExecutionHook(dispatchHook)
		}

		log.Println("[Bootstrap] Registering argument resolvers")
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

		log.Println("[Bootstrap] Registering return value handlers")
		httpPipeline.AddReturnValueHandler(
			&handler.RedirectReturnValueHandler{},
			&handler.BinaryReturnHandler{},
			&handler.StringReturnHandler{},
			&handler.JSONReturnHandler{},
			&handler.ErrorReturnHandler{},
		)

		log.Println("[Bootstrap] Registering interceptors")

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
			if t == nil {
				return fmt.Errorf("[Bootstrap] interceptor is nil")
			}

			if t.Kind() == reflect.Pointer && v.IsNil() {
				log.Printf("[Bootstrap] Created interceptor %s from the container", t.Elem().Name())

				inst, err := container.Resolve(t)
				if err != nil {
					return fmt.Errorf("[Bootstrap] failed to create interceptor: %w", err)
				}

				httpPipeline.AddInterceptor(inst.(core.Interceptor))
				continue
			}

			log.Printf("[Bootstrap] Using interceptor instance: %T", interceptor)
			httpPipeline.AddInterceptor(interceptor)
		}

		log.Println("[Bootstrap] Mounting HTTP adapter")

		// WebSocket Runtime 구성
		if config.WebSocketRegistry != nil && len(config.WebSocketRegistry.Registrations()) > 0 {
			wsRegistrations := config.WebSocketRegistry.Registrations()
			log.Println("[Bootstrap] Configuring WebSocket runtime")
			log.Printf("[Bootstrap] Configuring WebSocket routes (%d routes)", len(wsRegistrations))

			// WS 전용 ArgumentResolver 등록
			wsPipeline := buildWSPipeline(container, config.WebSocketRegistry, dispatchHook)

			wsRuntime = ws.NewRuntime(config.WebSocketRegistry, wsPipeline, config.HTTP.WebSocket)
			defer wsRuntime.Stop()

			// Echo Transport Hook으로 마운트
			wsMountHook := func(e any) {
				echoInstance, ok := e.(*echo.Echo)
				if !ok {
					return
				}
				for _, reg := range wsRegistrations {
					log.Printf("[Bootstrap] Registered WebSocket route: %s", reg.Path)
					echoInstance.GET(reg.Path, func(c echo.Context) error {
						wsRuntime.HandleConn(c.Response().Writer, c.Request(), reg)
						return nil
					})
				}
			}
			config.TransportHooks = append([]func(any){wsMountHook}, config.TransportHooks...)
		}

		// Echo Adapter
		server = httpEngine.NewServer(httpPipeline, config.Address, config.TransportHooks, *config.HTTP)
		server.Mount()

		log.Printf("[Bootstrap] Server listening on: %s", config.Address)
		httpErrCh = make(chan error, 1)
		go func() {
			if err := server.Start(); err != nil && err != http.ErrServerClosed {
				httpErrCh <- err
			}
		}()
	}

	// Consumer 컨트롤러 Warm-up
	if config.ConsumerRegistry != nil {
		log.Println("[Bootstrap] Warming up consumer controller dependencies")
		consumerRegistrations := config.ConsumerRegistry.Registrations()
		log.Printf("[Bootstrap] Configuring consumer routes (%d routes)", len(consumerRegistrations))
		var consumerTypes []reflect.Type
		for _, reg := range consumerRegistrations {
			log.Printf("[Bootstrap] Registered consumer route: %s", reg.Topic)
			consumerTypes = append(consumerTypes, reg.Meta.ControllerType)
		}
		if err := container.WarmUp(consumerTypes); err != nil {
			return fmt.Errorf("[Bootstrap] consumer controller warm-up failed: %w", err)
		}
	}

	// Kafka Read 옵션이 존재하면 Read를 Boot에 포함
	consumerStarted := false

	if config.Kafka != nil && config.Kafka.Read != nil && config.ConsumerRegistry != nil && len(config.ConsumerRegistry.Registrations()) > 0 {
		log.Println("[Bootstrap] Configuring Kafka consumer")
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
			return fmt.Errorf("[Bootstrap] Kafka consumer validation failed: %w", err)
		}

		forwardConsumerErrors("Kafka", runtime, consumerErrCh)
		go runtime.Start(context.Background())
		defer runtime.Stop()
		consumerStarted = true
	}

	// RabbitMQ 읽기 설정이 존재하면, 컨슈머 구성
	if config.RabbitMQ != nil && config.RabbitMQ.Read != nil && config.ConsumerRegistry != nil && len(config.ConsumerRegistry.Registrations()) > 0 {
		log.Println("[Bootstrap] Configuring RabbitMQ consumer")
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
			return fmt.Errorf("[Bootstrap] RabbitMQ consumer validation failed: %w", err)
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

		select {
		case err := <-httpErrCh:
			if err != nil {
				return err
			}
		case err := <-consumerErrCh:
			return err
		case err := <-customTransportErrCh:
			return err
		case <-httpShutdownSignals:
		}

		log.Println("[Bootstrap] Shutdown signal received. Starting graceful shutdown...")

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
			return fmt.Errorf("[Bootstrap] forced server shutdown: %v", err)
		}

		log.Println("[Bootstrap] Shutdown completed successfully")
	}

	// HTTP가 비활성화된 상태에서 이벤트 컨슈머만 실행 중이면 종료 신호를 기다린다.
	if config.HTTP == nil && (consumerStarted || customTransportErrCh != nil) {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(quit)
		select {
		case <-quit:
			log.Println("[Bootstrap] Shutdown signal received. Stopping runtimes...")
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

func joinPath(prefix, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("route path cannot be empty")
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	if prefix == "" {
		return path, nil
	}

	return prefix + path, nil
}

func assertNoAmbiguousRoute(method, newPath string, existing []string) error {
	newSegs := splitPathForValidation(newPath)

	for _, oldPath := range existing {
		oldSegs := splitPathForValidation(oldPath)

		// 서로 다른 segment length는 절대 겹치지 않음
		if len(newSegs) != len(oldSegs) {
			continue
		}

		// 각 segment가 충돌 없이 겹치는지(교집합 존재) 검사
		overlaps := true
		for i := range newSegs {
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
			return fmt.Errorf(
				"[Router] ambiguous route detected at boot: method=%s, new=%s conflicts with existing=%s",
				method, newPath, oldPath,
			)
		}
	}
	return nil
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
		select {
		case err := <-runtime.Errors():
			if err == nil {
				return
			}
			wrapped := fmt.Errorf("[Bootstrap] %s consumer runtime error: %w", name, err)
			select {
			case out <- wrapped:
			default:
				log.Printf("%v (could not forward because the consumer error channel is full)", wrapped)
			}
		case <-runtime.Done():
			return
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
	log.Printf("[Bootstrap] Spine version: %s", "v0.4.3")
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
