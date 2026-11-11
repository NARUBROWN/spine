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
	"github.com/NARUBROWN/spine/internal/configuration"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/endpoint"
	"github.com/NARUBROWN/spine/internal/event/consumer"
	"github.com/NARUBROWN/spine/internal/event/hook"
	eventPublish "github.com/NARUBROWN/spine/internal/event/publish"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
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
	resources, err := validate(config)
	if err != nil {
		return nil, err
	}
	return prepare(ctx, config, resources)
}

func prepare(ctx context.Context, config Config, resources []configuration.OpenAPIResource) (result *Application, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var openAPIDocument []byte
	openAPIResources := map[string]configuration.OpenAPIResource{}

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
			managed = eventPublish.NewPlatformPublisher(binding.Factory, publisher, binding.Options.PublisherErrorHistory)
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

	dispatchHook := &hook.EventDispatchHook{}
	if len(eventPublishers) > 0 {
		dispatcher, err := eventPublish.NewDefaultEventDispatcher(eventPublishers...)
		if err != nil {
			return nil, fmt.Errorf("[Bootstrap] failed to initialize event dispatcher: %w", err)
		}
		dispatchHook.Dispatcher = dispatcher
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

	if err := collectControllers(ctx, &config, container); err != nil {
		return nil, err
	}
	if config.HTTP != nil {
		if err := validateCollectedRoutes(config, resources); err != nil {
			return nil, err
		}
	}
	application.config = config
	if config.HTTP != nil && config.HTTP.OpenAPI.Enabled {
		description, describeErr := application.DescribeHTTP()
		if describeErr != nil {
			return nil, describeErr
		}
		openAPIDocument, err = endpoint.OpenAPI(description, config.HTTP.OpenAPI)
		if err != nil {
			return nil, err
		}
		for _, resource := range resources {
			openAPIResources[resource.Path] = resource
		}
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
			meta, err := spineRouter.Meta(route)
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

		httpPipeline.SetEventDispatch(dispatchHook)

		log.Println("[Bootstrap] Registering argument resolvers")
		httpPipeline.AddArgumentResolver(endpoint.Resolvers(config.Validator)...)
		httpPipeline.AddReturnValueHandler(endpoint.Returns()...)

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
		var upgrades []adapter.NetHTTPUpgradeRoute
		if config.WebSocketRegistry != nil && len(config.WebSocketRegistry.Registrations()) > 0 {
			wsRegistrations := config.WebSocketRegistry.Registrations()
			controllerTypes := make([]reflect.Type, 0, len(wsRegistrations))
			for _, registration := range wsRegistrations {
				if registration.Meta.ControllerType != nil {
					controllerTypes = append(controllerTypes, registration.Meta.ControllerType)
				}
			}
			if err := container.WarmUp(controllerTypes); err != nil {
				return nil, fmt.Errorf("[Bootstrap] WebSocket controller warm-up failed: %w", err)
			}
			log.Println("[Bootstrap] Configuring WebSocket runtime")
			log.Printf("[Bootstrap] Configuring WebSocket routes (%d routes)", len(wsRegistrations))

			// WS 전용 인자 리졸버 등록
			wsPipeline := buildWSPipeline(container, dispatchHook, resolvedWSInterceptors, config.Validator)

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
				upgrades = append(upgrades, adapter.NetHTTPUpgradeRoute{
					Path: reg.Path,
					Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						application.wsRuntime.HandleConn(w, r, reg)
					}),
				})
			}
		}

		httpConfig := adapter.Config{
			Options: serverOptions(*config.HTTP),
			Serve:   adapter.ServeOptions{TLS: config.HTTP.TLS},
			Execute: func(scope adapter.HTTPContext) (executionErr error) {
				request := transport.NewHTTPContext(scope, config.HTTP.MultipartMemoryBytes)
				defer func() { executionErr = errors.Join(executionErr, request.Cleanup()) }()
				if resource, ok := openAPIResources[request.Path()]; ok && (scope.Method() == http.MethodGet || scope.Method() == http.MethodHead) {
					scope.Writer().SetHeader("Content-Type", resource.ContentType)
					scope.Writer().SetHeader("Cache-Control", "no-cache")
					scope.Writer().SetHeader("X-Content-Type-Options", "nosniff")
					if scope.Method() == http.MethodHead {
						return scope.Writer().WriteStatus(http.StatusOK)
					}
					return scope.Writer().WriteString(http.StatusOK, resource.Body)
				}
				if len(openAPIDocument) > 0 && request.Path() == configuration.DocumentPath(config.HTTP.OpenAPI) && (scope.Method() == http.MethodGet || scope.Method() == http.MethodHead) {
					scope.Writer().SetHeader("Cache-Control", "no-cache")
					if scope.Method() == http.MethodHead {
						scope.Writer().SetHeader("Content-Type", "application/json")
						return scope.Writer().WriteStatus(http.StatusOK)
					}
					return scope.Writer().WriteJSON(http.StatusOK, append([]byte(nil), openAPIDocument...))
				}
				return httpPipeline.Execute(request)
			},
		}
		var server adapter.HTTPAdapter
		if len(upgrades) > 0 {
			server, err = config.HTTPAdapterFactory.(adapter.NetHTTPUpgradeFactory).NewWithNetHTTPUpgrades(httpConfig, upgrades)
		} else {
			server, err = config.HTTPAdapterFactory.New(httpConfig)
		}
		if !isNilHTTPValue(server) {
			application.server = server
		}
		if err != nil {
			return nil, fmt.Errorf("[Bootstrap] failed to configure HTTP adapter: %w", err)
		}
		if err := adapter.ValidateAdapter(server, config.HTTPAdapterFactory.Capabilities()); err != nil {
			return nil, err
		}
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
				buildConsumerPipeline(container, dispatchHook, config.Validator), binding.Options.ConsumerErrorHistory)
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
