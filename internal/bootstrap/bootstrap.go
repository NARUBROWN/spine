package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"

	adapter "github.com/NARUBROWN/spine/adapter"
	"github.com/NARUBROWN/spine/core"
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
	"github.com/NARUBROWN/spine/internal/transport"
	"github.com/NARUBROWN/spine/internal/ws"
	wsResolver "github.com/NARUBROWN/spine/internal/ws/resolver"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type Config struct {
	Address                string
	Constructors           []any
	Routes                 []spineRouter.RouteSpec
	Interceptors           []core.Interceptor
	ScopedInterceptors     []InterceptorBinding
	TransportHooks         []func(any)
	CustomTransports       []core.CustomTransport
	EnableGracefulShutdown bool
	ShutdownTimeout        time.Duration
	Kafka                  *boot.KafkaOptions
	RabbitMQ               *boot.RabbitMqOptions
	ConsumerRegistry       *consumer.Registry
	HTTP                   *boot.HTTPOptions
	HTTPAdapterFactory     adapter.Factory
	WebSocketRegistry      *ws.Registry
}

// InterceptorBinding은 글로벌 인터셉터를 기본 제공 전송 방식과 연결합니다.
// 부트스트랩 내부에서 사용하는 구조이며, 애플리케이션은
// App.InterceptorFor를 통해 등록합니다.
type InterceptorBinding struct {
	Interceptor core.Interceptor
	Scope       boot.InterceptorScope
}

type scopedInterceptor struct {
	interceptor core.Interceptor
	scope       boot.InterceptorScope
}

type interceptorIdentity struct {
	typeOf      reflect.Type
	pointer     uintptr
	placeholder bool
}

type containerFacade struct {
	container *container.Container
}

func (f *containerFacade) Resolve(t reflect.Type) (any, error) {
	return f.container.Resolve(t)
}

func Run(config Config) error {
	return RunContext(context.Background(), config)
}

func RunContext(ctx context.Context, config Config) error {
	if err := Validate(config); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	// 어떤 전송 방식도 초기화하거나 노출하기 전에 종료 신호를 구독합니다.
	// 정상 종료를 지원하는 HTTP 런타임과 HTTP가 없는 컨슈머/사용자 정의 런타임이 같은 채널을
	// 사용하므로 시작 중 신호도 잃지 않고 모든 반환 경로에서 구독을 해제합니다.
	var shutdownSignals chan os.Signal
	var shutdownRequested <-chan struct{}
	if requiresShutdownSignal(config) {
		shutdownSignals = make(chan os.Signal, 1)
		signal.Notify(shutdownSignals, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(shutdownSignals)
		requested := make(chan struct{})
		shutdownRequested = requested
		go func() {
			select {
			case <-shutdownSignals:
				cancelRun()
				close(requested)
			case <-runCtx.Done():
				close(requested)
			}
		}()
	}

	printBanner()

	log.Println("[Bootstrap] Initializing container")
	// 컨테이너 생성
	container := container.New()

	log.Printf("[Bootstrap] Registering constructors (%d)", len(config.Constructors))
	// 생성자 등록(HTTP/컨슈머 공통)
	for _, constructor := range config.Constructors {
		log.Printf("[Bootstrap] Registering constructor: %T", constructor)
		if err := container.RegisterConstructor(constructor); err != nil {
			return err
		}
	}

	// 이벤트 발행기 모음(Kafka/RabbitMQ 등의 설정에 따라 채워짐)
	var eventPublishers []eventPublish.EventPublisher

	// Kafka 쓰기 설정이 있으면 퍼블리셔 구성
	if config.Kafka != nil && config.Kafka.Write != nil {
		log.Println("[Bootstrap] Configuring Kafka publisher")

		kafkaPublisher, err := kafka.NewKafkaPublisher(&boot.KafkaOptions{
			Brokers:                config.Kafka.Brokers,
			TLS:                    config.Kafka.TLS,
			Dialer:                 config.Kafka.Dialer,
			Transport:              config.Kafka.Transport,
			AllowInsecureTransport: config.Kafka.AllowInsecureTransport,
			ConsumerRetry:          config.Kafka.ConsumerRetry,
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

	// RabbitMQ 쓰기 설정이 있으면 퍼블리셔 구성
	if config.RabbitMQ != nil && config.RabbitMQ.Write != nil {
		log.Println("[Bootstrap] Configuring RabbitMQ publisher")

		rabbitmqWriter, err := rabbitmq.NewRabbitMqWriter(boot.RabbitMqOptions{
			URL:                    config.RabbitMQ.URL,
			AllowInsecureTransport: config.RabbitMQ.AllowInsecureTransport,
			ConsumerRetry:          config.RabbitMQ.ConsumerRetry,
			PublisherRetry:         config.RabbitMQ.PublisherRetry,
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

	// 실행 후 훅에서 사용할 공통 디스패처(퍼블리셔가 없으면 nil 유지)
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

	var server adapter.HTTPAdapter
	var httpListener *managedHTTPListener
	var httpErrCh chan error
	var consumerErrCh chan error
	var customTransportErrCh chan error
	var wsRuntime *ws.Runtime
	var shutdownHTTPServer func(context.Context) error
	var consumerRuntimes []*consumer.Runtime
	var initializedCustomTransports []core.CustomTransport
	shutdownTimeout := config.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = 10 * time.Second
	}

	stopCustomTransportOnce := sync.Once{}
	stopCustomTransports := func(ctx context.Context) {
		stopCustomTransportOnce.Do(func() {
			for i := len(initializedCustomTransports) - 1; i >= 0; i-- {
				transport := initializedCustomTransports[i]
				if err := transport.Stop(ctx); err != nil {
					log.Printf("[Bootstrap] failed to stop custom transport: %v", err)
				}
			}
		})
	}

	cleanupOnce := sync.Once{}
	var cleanupErr error
	cleanup := func() error {
		cleanupOnce.Do(func() {
			// 먼저 ingress를 닫아 종료 중 새 HTTP/WS 작업이 시작되지 않게 합니다.
			if wsRuntime != nil {
				wsRuntime.Stop()
			}
			if shutdownHTTPServer != nil {
				ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
				if err := shutdownHTTPServer(ctx); err != nil {
					cleanupErr = errors.Join(cleanupErr, err)
					log.Printf("[Bootstrap] failed to shut down HTTP server: %v", err)
				}
				cancel()
			}

			// Consumer Stop은 모든 worker/ACK/NACK/reader 종료가 끝날 때까지 대기하는 계약입니다.
			for i := len(consumerRuntimes) - 1; i >= 0; i-- {
				consumerRuntimes[i].Stop()
			}

			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			stopCustomTransports(ctx)
			cancel()
		})
		return cleanupErr
	}
	defer cleanup()

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
			initializedCustomTransports = append(initializedCustomTransports, transport)
		}

		customTransportErrCh = make(chan error, len(config.CustomTransports))
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
		// 라우터 생성 및 라우트 등록
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
		// 미리 초기화할 컴포넌트
		if err := container.WarmUp(router.ControllerTypes()); err != nil {
			return fmt.Errorf("[Bootstrap] HTTP controller warm-up failed: %w", err)
		}

		log.Println("[Bootstrap] Building execution pipeline")
		httpInvoker := invoker.NewInvoker(container)
		httpPipeline := pipeline.NewPipeline(router, httpInvoker)

		// HTTP 실행 후 훅: 도메인 이벤트 발행(퍼블리셔가 있는 경우에만)
		if dispatchHook != nil {
			httpPipeline.AddPostExecutionHook(dispatchHook)
		}

		log.Println("[Bootstrap] Registering argument resolvers")
		httpPipeline.AddArgumentResolver(
			// 표준 컨텍스트 리졸버
			&resolver.StdContextResolver{},

			// Spine 컨트롤러 컨텍스트 뷰
			&resolver.ControllerContextResolver{},

			// 헤더 리졸버
			&resolver.HeaderResolver{},

			// 경로 리졸버들
			&resolver.PathIntResolver{},
			&resolver.PathStringResolver{},
			&resolver.PathBooleanResolver{},

			// 쿼리의 의미 타입 리졸버들
			&resolver.PaginationResolver{},
			&resolver.QueryValuesResolver{},

			// 요청 본문 리졸버
			&resolver.DTOResolver{},

			// 폼 DTO(멀티파트/폼)
			&resolver.FormDTOResolver{},

			// 멀티파트 파일
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

		resolvedHTTPInterceptors, resolvedWSInterceptors, err := resolveGlobalInterceptors(container, config)
		if err != nil {
			return err
		}
		for _, interceptor := range resolvedHTTPInterceptors {
			httpPipeline.AddInterceptor(interceptor)
		}

		log.Println("[Bootstrap] Mounting HTTP adapter")

		// WebSocket 런타임은 업그레이드 이후의 연결과 메시지를 계속 소유합니다.
		var upgrades []adapter.UpgradeRoute
		if config.WebSocketRegistry != nil && len(config.WebSocketRegistry.Registrations()) > 0 {
			wsRegistrations := config.WebSocketRegistry.Registrations()
			log.Println("[Bootstrap] Configuring WebSocket runtime")
			log.Printf("[Bootstrap] Configuring WebSocket routes (%d routes)", len(wsRegistrations))

			// WS 전용 인자 리졸버 등록
			wsPipeline := buildWSPipeline(container, config.WebSocketRegistry, dispatchHook, resolvedWSInterceptors)

			wsRuntime = ws.NewRuntime(
				config.WebSocketRegistry,
				wsPipeline,
				config.HTTP.WebSocket,
				webSocketHandshakeInterceptors(resolvedWSInterceptors)...,
			)

			for _, reg := range wsRegistrations {
				log.Printf("[Bootstrap] Registered WebSocket route: %s", reg.Path)
				upgrades = append(upgrades, adapter.UpgradeRoute{
					Path: reg.Path,
					Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						wsRuntime.HandleConn(w, r, reg)
					}),
				})
			}
		}

		server, err = config.HTTPAdapterFactory.New(adapter.Config{
			Options: serverOptions(*config.HTTP),
			Execute: func(scope adapter.HTTPContext) error {
				return httpPipeline.Execute(transport.NewHTTPContext(scope))
			},
			Upgrades:    upgrades,
			NativeHooks: config.TransportHooks,
		})
		if err != nil {
			return fmt.Errorf("[Bootstrap] failed to configure HTTP adapter: %w", err)
		}
		if isNilHTTPValue(server) {
			return fmt.Errorf("%w: factory returned a nil adapter", adapter.ErrInvalidAdapter)
		}

		httpErrCh = make(chan error, 1)
		shutdownHTTPServer = func(ctx context.Context) error {
			err := server.Shutdown(ctx)
			if httpListener != nil {
				// Serve goroutine으로 전달되기 전에 종료되어도 열린 리스너를 남기지 않습니다.
				return errors.Join(err, httpListener.Close())
			}
			return err
		}
		if isNilHTTPValue(server.Handler()) {
			return fmt.Errorf("%w: factory returned a nil handler", adapter.ErrInvalidAdapter)
		}
	}

	// 컨슈머 컨트롤러 미리 초기화
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

	// Kafka 읽기 설정이 있으면 읽기 작업을 부트 절차에 포함
	consumerStarted := false

	if config.Kafka != nil && config.Kafka.Read != nil && config.ConsumerRegistry != nil && len(config.ConsumerRegistry.Registrations()) > 0 {
		log.Println("[Bootstrap] Configuring Kafka consumer")
		if consumerErrCh == nil {
			consumerErrCh = make(chan error, 1)
		}

		factory := kafka.NewRunnerFactory(boot.KafkaOptions{
			Brokers:                config.Kafka.Brokers,
			TLS:                    config.Kafka.TLS,
			Dialer:                 config.Kafka.Dialer,
			Transport:              config.Kafka.Transport,
			AllowInsecureTransport: config.Kafka.AllowInsecureTransport,
			ConsumerRetry:          config.Kafka.ConsumerRetry,
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

		if err := runtime.ValidateWithRetry(runCtx); err != nil {
			if errors.Is(err, context.Canceled) {
				return cleanup()
			}
			return fmt.Errorf("[Bootstrap] Kafka consumer validation failed: %w", err)
		}

		forwardConsumerErrors("Kafka", runtime, consumerErrCh)
		consumerRuntimes = append(consumerRuntimes, runtime)
		consumerStarted = true
	}

	// RabbitMQ 읽기 설정이 존재하면, 컨슈머 구성
	if config.RabbitMQ != nil && config.RabbitMQ.Read != nil && config.ConsumerRegistry != nil && len(config.ConsumerRegistry.Registrations()) > 0 {
		log.Println("[Bootstrap] Configuring RabbitMQ consumer")
		failurePolicy := config.RabbitMQ.Read.EffectiveFailurePolicy()
		log.Printf("[Bootstrap] RabbitMQ consumer failure policy: %s", failurePolicy)
		if config.RabbitMQ.Read.DeadLetter != nil {
			log.Printf("[Bootstrap] RabbitMQ dead-letter exchange: %s", config.RabbitMQ.Read.DeadLetter.Exchange)
		} else if failurePolicy == boot.RabbitMqFailureReject {
			log.Println("[Bootstrap] Warning: rejected RabbitMQ messages may be discarded because no dead-letter exchange is configured")
		}
		if consumerErrCh == nil {
			consumerErrCh = make(chan error, 1)
		}

		factory := rabbitmq.NewRunnerFactory(boot.RabbitMqOptions{
			URL:                    config.RabbitMQ.URL,
			AllowInsecureTransport: config.RabbitMQ.AllowInsecureTransport,
			ConsumerRetry:          config.RabbitMQ.ConsumerRetry,
			Read: &boot.RabbitMqReadOptions{
				Exchange:       config.RabbitMQ.Read.Exchange,
				PrefetchCount:  config.RabbitMQ.Read.PrefetchCount,
				FailurePolicy:  config.RabbitMQ.Read.FailurePolicy,
				DeadLetter:     config.RabbitMQ.Read.DeadLetter,
				RequeueOnError: config.RabbitMQ.Read.RequeueOnError,
			},
		})

		consumerPipeline := buildConsumerPipeline(container, config.ConsumerRegistry, dispatchHook)

		runtime := consumer.NewRuntime(
			config.ConsumerRegistry,
			factory,
			consumerPipeline,
		)

		if err := runtime.ValidateWithRetry(runCtx); err != nil {
			if errors.Is(err, context.Canceled) {
				return cleanup()
			}
			return fmt.Errorf("[Bootstrap] RabbitMQ consumer validation failed: %w", err)
		}

		forwardConsumerErrors("RabbitMQ", runtime, consumerErrCh)
		consumerRuntimes = append(consumerRuntimes, runtime)
		consumerStarted = true
	}

	// expose 단계: 모든 DI warm-up과 broker runtime validation이 성공한 뒤에만
	// 장기 실행 runtime과 마지막으로 HTTP listener를 시작합니다.
	if runCtx.Err() != nil {
		return cleanup()
	}
	for _, runtime := range consumerRuntimes {
		go runtime.Start(runCtx)
	}
	for _, transport := range initializedCustomTransports {
		transport := transport
		go func() {
			if err := transport.Start(); err != nil {
				customTransportErrCh <- err
			}
		}()
	}
	if server != nil {
		if runCtx.Err() != nil {
			return cleanup()
		}
		address := config.Address
		if address == "" {
			// 기존 http.Server.ListenAndServe의 빈 주소 기본값을 유지합니다.
			address = ":http"
		}
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return fmt.Errorf("[Bootstrap] failed to listen on %s: %w", address, err)
		}
		httpListener = &managedHTTPListener{Listener: listener}
		if runCtx.Err() != nil {
			_ = httpListener.Close()
			return cleanup()
		}
		log.Printf("[Bootstrap] Server listening on: %s", listener.Addr())
		go func() {
			err := server.Serve(httpListener)
			if err == nil {
				err = fmt.Errorf("%w: Serve returned nil", adapter.ErrInvalidAdapter)
			}
			if !errors.Is(err, http.ErrServerClosed) {
				httpErrCh <- err
			}
		}()
	}

	if config.HTTP != nil {
		// 정상 종료 기능 비활성화: 서버가 종료될 때까지 대기
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
			case <-runCtx.Done():
				return cleanup()
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
		case <-shutdownRequested:
		}

		log.Println("[Bootstrap] Shutdown signal received. Starting graceful shutdown...")
		if err := cleanup(); err != nil {
			return fmt.Errorf("[Bootstrap] forced server shutdown: %v", err)
		}

		log.Println("[Bootstrap] Shutdown completed successfully")
	}

	// HTTP가 비활성화된 상태에서 이벤트 컨슈머만 실행 중이면 종료 신호를 기다린다.
	if config.HTTP == nil && (consumerStarted || customTransportErrCh != nil) {
		select {
		case <-shutdownRequested:
			log.Println("[Bootstrap] Shutdown signal received. Stopping runtimes...")
		case err := <-consumerErrCh:
			return err
		case err := <-customTransportErrCh:
			return err
		}

		_ = cleanup()
	}

	return nil
}

func requiresShutdownSignal(config Config) bool {
	if config.HTTP != nil {
		return config.EnableGracefulShutdown
	}
	if len(config.CustomTransports) > 0 {
		return true
	}
	if config.ConsumerRegistry == nil || len(config.ConsumerRegistry.Registrations()) == 0 {
		return false
	}
	return (config.Kafka != nil && config.Kafka.Read != nil) ||
		(config.RabbitMQ != nil && config.RabbitMQ.Read != nil)
}

// Validate는 네트워크에 접근하지 않고 사전 검증을 수행하며,
// 사용자가 조치할 수 있는 모든 설정 문제를 하나의 오류로 반환합니다.
func Validate(config Config) error {
	var issues []boot.ConfigIssue
	if config.ShutdownTimeout < 0 {
		issues = append(issues, boot.ConfigIssue{
			Path:    "ShutdownTimeout",
			Code:    "SHUTDOWN_TIMEOUT_INVALID",
			Message: "Shutdown timeout cannot be negative.",
			Hint:    "Use zero for the default or a positive duration.",
		})
	}
	if config.Kafka != nil {
		issues = append(issues, config.Kafka.ValidateIssues("Kafka")...)
	}
	if config.RabbitMQ != nil {
		issues = append(issues, config.RabbitMQ.ValidateIssues("RabbitMQ")...)
	}

	if config.HTTP == nil {
		if config.WebSocketRegistry != nil && len(config.WebSocketRegistry.Registrations()) > 0 {
			issues = append(issues, boot.ConfigIssue{
				Path: "HTTP", Code: "WEBSOCKET_HTTP_REQUIRED",
				Message: "WebSocket routes require the HTTP runtime.",
				Hint:    "Configure boot.Options.HTTP before registering WebSocket routes.",
			})
		}
	} else {
		prefix := config.HTTP.GlobalPrefix
		if prefix != "" && (!strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, ":*")) {
			issues = append(issues, boot.ConfigIssue{
				Path: "HTTP.GlobalPrefix", Code: "HTTP_GLOBAL_PREFIX_INVALID",
				Message: "HTTP global prefix must start with '/' and cannot contain parameters or wildcards.",
				Hint:    "Use a static prefix such as /api/v1.",
			})
		}
		issues = append(issues, config.HTTP.ValidateIssues("HTTP")...)
	}
	for i, route := range config.Routes {
		if _, err := spineRouter.NewHandlerMeta(route.Handler); err != nil {
			issues = append(issues, boot.ConfigIssue{
				Path:    fmt.Sprintf("Routes[%d].Handler", i),
				Code:    "HTTP_ROUTE_HANDLER_INVALID",
				Message: fmt.Sprintf("HTTP route handler is invalid: %v", err),
				Hint:    "Provide a non-nil controller method expression such as (*Controller).Handle.",
			})
		}
	}

	bindings := make([]InterceptorBinding, 0, len(config.Interceptors)+len(config.ScopedInterceptors))
	for _, interceptor := range config.Interceptors {
		bindings = append(bindings, InterceptorBinding{Interceptor: interceptor, Scope: boot.InterceptorAll})
	}
	bindings = append(bindings, config.ScopedInterceptors...)
	for i, binding := range bindings {
		path := fmt.Sprintf("Interceptors[%d]", i)
		if reflect.TypeOf(binding.Interceptor) == nil {
			issues = append(issues, boot.ConfigIssue{Path: path, Code: "INTERCEPTOR_NIL", Message: "Global interceptor is nil.", Hint: "Remove the nil entry or provide an interceptor instance."})
		}
		if binding.Scope == 0 || binding.Scope&^boot.InterceptorAll != 0 {
			issues = append(issues, boot.ConfigIssue{Path: path + ".Scope", Code: "INTERCEPTOR_SCOPE_INVALID", Message: "Interceptor transport scope is invalid.", Hint: "Use boot.InterceptorHTTP, boot.InterceptorWebSocket, or boot.InterceptorAll."})
		}
	}
	if err := boot.ValidationError(issues...); err != nil {
		return err
	}
	if config.HTTP != nil {
		return adapter.ValidateFactory(config.HTTPAdapterFactory, adapter.Requirements{
			WebSocketUpgrade: config.WebSocketRegistry != nil && len(config.WebSocketRegistry.Registrations()) > 0,
			NativeHooks:      len(config.TransportHooks) > 0,
		})
	}
	return nil
}

func serverOptions(options boot.HTTPOptions) adapter.ServerOptions {
	return adapter.ServerOptions{
		DisableRecover:    options.DisableRecover,
		ReadHeaderTimeout: options.ReadHeaderTimeout,
		ReadTimeout:       options.ReadTimeout,
		WriteTimeout:      options.WriteTimeout,
		IdleTimeout:       options.IdleTimeout,
		MaxHeaderBytes:    options.MaxHeaderBytes,
		MaxBodyBytes:      options.MaxBodyBytes,
	}
}

func isNilHTTPValue(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func resolveGlobalInterceptors(ctr *container.Container, config Config) ([]core.Interceptor, []core.Interceptor, error) {
	bindings := make([]InterceptorBinding, 0, len(config.Interceptors)+len(config.ScopedInterceptors))
	for _, interceptor := range config.Interceptors {
		bindings = append(bindings, InterceptorBinding{Interceptor: interceptor, Scope: boot.InterceptorAll})
	}
	bindings = append(bindings, config.ScopedInterceptors...)

	ordered := make([]scopedInterceptor, 0, len(bindings))
	seen := make(map[interceptorIdentity]int)
	for i, binding := range bindings {
		t := reflect.TypeOf(binding.Interceptor)
		if t == nil {
			return nil, nil, fmt.Errorf("[Bootstrap] interceptor[%d] is nil", i)
		}
		if binding.Scope == 0 || binding.Scope&^boot.InterceptorAll != 0 {
			return nil, nil, fmt.Errorf("[Bootstrap] interceptor[%d] has invalid transport scope %d", i, binding.Scope)
		}
		if identity, identifiable := globalInterceptorIdentity(binding.Interceptor); identifiable {
			if index, ok := seen[identity]; ok {
				ordered[index].scope |= binding.Scope
				continue
			}
			seen[identity] = len(ordered)
		}
		ordered = append(ordered, scopedInterceptor{interceptor: binding.Interceptor, scope: binding.Scope})
	}

	resolvedHTTP := make([]core.Interceptor, 0, len(ordered))
	resolvedWS := make([]core.Interceptor, 0, len(ordered))
	for _, binding := range ordered {
		interceptor := binding.interceptor
		v := reflect.ValueOf(interceptor)
		t := reflect.TypeOf(interceptor)
		if t.Kind() == reflect.Pointer && v.IsNil() {
			log.Printf("[Bootstrap] Created interceptor %s from the container", t.Elem().Name())
			inst, err := ctr.Resolve(t)
			if err != nil {
				return nil, nil, fmt.Errorf("[Bootstrap] failed to create interceptor: %w", err)
			}
			interceptor = inst.(core.Interceptor)
		} else {
			log.Printf("[Bootstrap] Using interceptor instance: %T", interceptor)
		}
		if binding.scope&boot.InterceptorHTTP != 0 {
			resolvedHTTP = append(resolvedHTTP, interceptor)
		}
		if binding.scope&boot.InterceptorWebSocket != 0 {
			resolvedWS = append(resolvedWS, interceptor)
		}
	}
	return resolvedHTTP, resolvedWS, nil
}

func webSocketHandshakeInterceptors(interceptors []core.Interceptor) []core.WebSocketHandshakeInterceptor {
	handshakeInterceptors := make([]core.WebSocketHandshakeInterceptor, 0, len(interceptors))
	for _, interceptor := range interceptors {
		if handshakeInterceptor, ok := interceptor.(core.WebSocketHandshakeInterceptor); ok {
			handshakeInterceptors = append(handshakeInterceptors, handshakeInterceptor)
		}
	}
	return handshakeInterceptors
}

// globalInterceptorIdentity는 Go에서 실제 인스턴스의 정체성을 보존할 수 있을 때만
// 식별 정보를 반환합니다. 값이 같은 비포인터 인터셉터도 별도 등록일 수 있으므로
// 의도적으로 중복 제거하지 않습니다.
func globalInterceptorIdentity(interceptor core.Interceptor) (interceptorIdentity, bool) {
	t := reflect.TypeOf(interceptor)
	if t == nil || t.Kind() != reflect.Pointer {
		return interceptorIdentity{}, false
	}
	v := reflect.ValueOf(interceptor)
	if v.IsNil() {
		return interceptorIdentity{typeOf: t, placeholder: true}, true
	}
	return interceptorIdentity{typeOf: t, pointer: v.Pointer()}, true
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

		// 경로 조각 수가 다르면 절대 겹치지 않음
		if len(newSegs) != len(oldSegs) {
			continue
		}

		// 각 경로 조각이 충돌 없이 겹치는지(교집합 존재) 검사
		overlaps := true
		for i := range newSegs {
			a := newSegs[i]
			b := oldSegs[i]

			aParam := isPathParam(a)
			bParam := isPathParam(b)

			// 둘 다 리터럴인데 값이 다르면 이 위치에서 교집합이 사라짐
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
		err := waitConsumerError(runtime.Errors(), runtime.Done())
		if err == nil {
			return
		}
		wrapped := fmt.Errorf("[Bootstrap] %s consumer runtime error: %w", name, err)
		select {
		case out <- wrapped:
		default:
			log.Printf("%v (could not forward because the consumer error channel is full)", wrapped)
		}
	}()
}

// waitConsumerError는 치명적 종료 시 Errors가 Done보다 먼저 기록된다는 Runtime 계약을
// 반영합니다. 두 채널이 동시에 준비된 경우에도 Done을 정상 종료로 오인하지 않습니다.
func waitConsumerError(errors <-chan error, done <-chan struct{}) error {
	select {
	case err := <-errors:
		return err
	case <-done:
		select {
		case err := <-errors:
			return err
		default:
			return nil
		}
	}
}

const (
	spineVersion = "v0.5.2"
	spineBanner  = `
________       _____             
__  ___/__________(_)___________ 
_____ \___  __ \_  /__  __ \  _ \
____/ /__  /_/ /  / _  / / /  __/
/____/ _  .___//_/  /_/ /_/\___/ 
       /_/        
`
)

func printBanner() {
	fmt.Print(spineBanner)
	log.Printf("[Bootstrap] Spine version: %s", spineVersion)
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
	interceptors []core.Interceptor,
) *pipeline.Pipeline {
	wsRouter := spineRouter.NewRouter()
	for _, reg := range registry.Registrations() {
		wsRouter.Register("WS", reg.Path, reg.Meta)
	}

	wsInvoker := invoker.NewInvoker(container)
	wsPipeline := pipeline.NewPipeline(wsRouter, wsInvoker)
	for _, interceptor := range interceptors {
		wsPipeline.AddInterceptor(interceptor)
	}

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
