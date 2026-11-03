package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strings"

	eventadapter "github.com/NARUBROWN/spine/contract/event"
	adapter "github.com/NARUBROWN/spine/contract/http"
	wsadapter "github.com/NARUBROWN/spine/contract/websocket"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/event/consumer"
	"github.com/NARUBROWN/spine/internal/event/hook"
	eventPublish "github.com/NARUBROWN/spine/internal/event/publish"
	"github.com/NARUBROWN/spine/internal/handler"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/NARUBROWN/spine/internal/resolver"
	spineRouter "github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/internal/transport"
	"github.com/NARUBROWN/spine/internal/ws"
)

// Prepare는 실행 작업이나 리스너를 시작하지 않고 앱 자원을 준비합니다.
func Prepare(ctx context.Context, config Config) (*Application, error) {
	if isNilHTTPValue(ctx) {
		return nil, ErrInvalidContext
	}
	config = snapshotConfig(config)
	if err := Validate(config); err != nil {
		return nil, err
	}
	return prepare(ctx, config)
}

func prepare(ctx context.Context, config Config) (result *Application, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	application := &Application{config: config, closeDone: make(chan struct{})}
	ready := false
	defer func() {
		if !ready {
			if closeErr := application.closeWithTimeout(); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
		}
	}()
	log.Println("[Bootstrap] Initializing container")
	// 컨테이너 생성
	container := container.New()

	log.Printf("[Bootstrap] Registering constructors (%d)", len(config.Constructors))
	// 생성자 등록(HTTP/컨슈머 공통)
	for _, constructor := range config.Constructors {
		log.Printf("[Bootstrap] Registering constructor: %T", constructor)
		if err := container.RegisterConstructor(constructor); err != nil {
			return nil, err
		}
	}

	var eventPublishers []eventPublish.EventPublisher
	for _, binding := range config.EventAdapters {
		if !binding.Factory.Capabilities().Publish {
			continue
		}
		publisher, err := binding.Factory.NewPublisher(ctx)
		var managed *eventPublish.PlatformPublisher
		if !isNilHTTPValue(publisher) {
			managed = eventPublish.NewPlatformPublisher(binding.Factory, publisher)
			application.publishers = append(application.publishers, publisherCloser{name: binding.Name, close: managed.Close})
		}
		if err != nil {
			if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("[Bootstrap] %s publisher initialization failed: %w", binding.Name, err)
		}
		if isNilHTTPValue(publisher) {
			return nil, fmt.Errorf("%w: %s returned a nil publisher", eventadapter.ErrInvalidAdapter, binding.Name)
		}
		eventPublishers = append(eventPublishers, managed)
	}

	// 실행 후 훅에서 사용할 공통 디스패처(퍼블리셔가 없으면 nil 유지)
	var dispatchHook *hook.EventDispatchHook
	if len(eventPublishers) > 0 {
		dispatcher, err := eventPublish.NewDefaultEventDispatcher(eventPublishers...)
		if err != nil {
			return nil, fmt.Errorf("[Bootstrap] failed to initialize event dispatcher: %w", err)
		}
		dispatchHook = &hook.EventDispatchHook{
			Dispatcher: dispatcher,
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(config.CustomTransports) > 0 {
		log.Printf("[Bootstrap] Initializing custom transports (%d)", len(config.CustomTransports))
		facade := &containerFacade{container: container}

		for i, transport := range config.CustomTransports {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if isNilHTTPValue(transport) {
				return nil, fmt.Errorf("[Bootstrap] custom transport[%d] is nil", i)
			}
			if err := transport.Init(facade); err != nil {
				return nil, fmt.Errorf("[Bootstrap] custom transport initialization failed: %w", err)
			}
			application.transports = append(application.transports, transport)
		}

	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if config.HTTP != nil {
		prefix := config.HTTP.GlobalPrefix
		if prefix != "" {
			if !strings.HasPrefix(prefix, "/") {
				return nil, fmt.Errorf("HTTP global prefix must start with '/'")
			}
			if strings.Contains(prefix, ":") {
				return nil, fmt.Errorf("path parameters are not allowed in the HTTP global prefix")
			}
			if strings.Contains(prefix, "*") {
				return nil, fmt.Errorf("wildcards are not allowed in the HTTP global prefix")
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
				return nil, err
			}

			resolved := make([]core.Interceptor, len(route.Interceptors))
			for i, interceptor := range route.Interceptors {
				interceptorType := reflect.TypeOf(interceptor)
				if interceptorType == nil {
					return nil, fmt.Errorf("[Bootstrap] route interceptor[%d] is nil", i)
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
						return nil, fmt.Errorf("[Bootstrap] failed to create route interceptor: %w", err)
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
				return nil, err
			}
			log.Printf("[Bootstrap] Registered HTTP route: (%s) %s", route.Method, fullPath)

			if err := assertNoAmbiguousRoute(route.Method, fullPath, registeredPathsByMethod[route.Method]); err != nil {
				return nil, err
			}
			registeredPathsByMethod[route.Method] = append(registeredPathsByMethod[route.Method], fullPath)

			router.Register(route.Method, fullPath, meta)
		}

		log.Println("[Bootstrap] Warming up controller dependencies")
		// 미리 초기화할 컴포넌트
		if err := container.WarmUp(router.ControllerTypes()); err != nil {
			return nil, fmt.Errorf("[Bootstrap] HTTP controller warm-up failed: %w", err)
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
			&resolver.DTOResolver{Validator: config.Validator},

			// 폼 DTO(멀티파트/폼)
			&resolver.FormDTOResolver{Validator: config.Validator},

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
			return nil, err
		}
		for _, interceptor := range resolvedHTTPInterceptors {
			httpPipeline.AddInterceptor(interceptor)
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}
		log.Println("[Bootstrap] Mounting HTTP adapter")

		// WebSocket 런타임은 업그레이드 이후의 연결과 메시지를 계속 소유합니다.
		var upgrades []adapter.UpgradeRoute
		if config.WebSocketRegistry != nil && len(config.WebSocketRegistry.Registrations()) > 0 {
			wsRegistrations := config.WebSocketRegistry.Registrations()
			log.Println("[Bootstrap] Configuring WebSocket runtime")
			log.Printf("[Bootstrap] Configuring WebSocket routes (%d routes)", len(wsRegistrations))

			// WS 전용 인자 리졸버 등록
			wsPipeline := buildWSPipeline(container, config.WebSocketRegistry, dispatchHook, resolvedWSInterceptors, config.Validator)

			wsAdapter, err := config.WebSocketAdapterFactory.New(ws.AdapterOptions(config.HTTP.WebSocket))
			if err != nil {
				return nil, fmt.Errorf("[Bootstrap] failed to configure WebSocket adapter: %w", err)
			}
			if err := wsadapter.ValidateAdapter(wsAdapter); err != nil {
				return nil, err
			}
			application.wsRuntime = ws.NewRuntime(
				wsPipeline,
				wsAdapter,
				config.HTTP.WebSocket,
				webSocketHandshakeInterceptors(resolvedWSInterceptors)...,
			)

			for _, reg := range wsRegistrations {
				log.Printf("[Bootstrap] Registered WebSocket route: %s", reg.Path)
				upgrades = append(upgrades, adapter.UpgradeRoute{
					Path: reg.Path,
					Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						application.wsRuntime.HandleConn(w, r, reg)
					}),
				})
			}
		}

		server, err := config.HTTPAdapterFactory.New(adapter.Config{
			Options: serverOptions(*config.HTTP),
			Execute: func(scope adapter.HTTPContext) error {
				return httpPipeline.Execute(transport.NewHTTPContext(scope))
			},
			Upgrades: upgrades,
		})
		if !isNilHTTPValue(server) {
			application.server = server
		}
		if err != nil {
			return nil, fmt.Errorf("[Bootstrap] failed to configure HTTP adapter: %w", err)
		}
		if application.server == nil {
			return nil, fmt.Errorf("%w: factory returned a nil adapter", adapter.ErrInvalidAdapter)
		}
		nativeHandler := server.Handler()
		if isNilHTTPValue(nativeHandler) {
			return nil, fmt.Errorf("%w: factory returned a nil handler", adapter.ErrInvalidAdapter)
		}
		application.handler = newPreparedHandler(nativeHandler)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
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
			return nil, fmt.Errorf("[Bootstrap] consumer controller warm-up failed: %w", err)
		}
	}

	if config.ConsumerRegistry != nil && len(config.ConsumerRegistry.Registrations()) > 0 {
		for _, binding := range config.EventAdapters {
			if !binding.Factory.Capabilities().Consume {
				continue
			}
			runtime := consumer.NewRuntime(config.ConsumerRegistry,
				consumer.NewPlatformFactory(binding.Factory, binding.Options),
				buildConsumerPipeline(container, config.ConsumerRegistry, dispatchHook, config.Validator))
			application.consumers = append(application.consumers, consumerRuntime{name: binding.Name, runtime: runtime})
			if err := runtime.PrepareWithRetry(ctx); err != nil {
				if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
					return nil, ctx.Err()
				}
				return nil, fmt.Errorf("[Bootstrap] %s consumer validation failed: %w", binding.Name, err)
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ready = true
	return application, nil
}
