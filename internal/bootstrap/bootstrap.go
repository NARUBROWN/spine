package bootstrap

import (
	"context"
	"errors"
	"fmt"
	publicWS "github.com/NARUBROWN/spine/pkg/ws"
	"log"
	"reflect"
	"slices"
	"strings"
	"time"

	eventadapter "github.com/NARUBROWN/spine/contract/event"
	adapter "github.com/NARUBROWN/spine/contract/http"
	validation "github.com/NARUBROWN/spine/contract/validator"
	wsadapter "github.com/NARUBROWN/spine/contract/websocket"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/configuration"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/endpoint"
	"github.com/NARUBROWN/spine/internal/event/consumer"
	eventResolver "github.com/NARUBROWN/spine/internal/event/consumer/resolver"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/NARUBROWN/spine/internal/resolver"
	spineRouter "github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/internal/ws"
	wsResolver "github.com/NARUBROWN/spine/internal/ws/resolver"
)

type Config struct {
	Address                      string
	Constructors                 []any
	ControllerConstructors       []any
	ComponentIssues              []configuration.ConfigIssue
	TypedComponents              bool
	Routes                       []spineRouter.RouteSpec
	Interceptors                 []core.Interceptor
	ScopedInterceptors           []InterceptorBinding
	WebSocketInterceptors        []core.WebSocketInterceptor
	WebSocketUpgradeInterceptors []core.WebSocketUpgradeInterceptor
	CustomTransports             []core.CustomTransport
	ShutdownHooks                []any
	EnableGracefulShutdown       bool
	ShutdownTimeout              time.Duration
	EventAdapters                []EventAdapterBinding
	ConsumerRegistry             *consumer.Registry
	HTTP                         *configuration.HTTPOptions
	HTTPAdapterFactory           adapter.Factory
	WebSocketAdapterFactory      wsadapter.Factory
	WebSocketRegistry            *ws.Registry
	Validator                    validation.Validator
	ValidatorConfigured          bool
	RoutesCollected              bool
}

// EventAdapterBinding은 구현과 공통 실행 정책을 연결하는 준비 설정입니다.
type EventAdapterBinding struct {
	Name    string
	Factory eventadapter.Factory
	Options configuration.EventOptions
}

// InterceptorBinding은 글로벌 인터셉터를 기본 제공 전송 방식과 연결합니다.
// 부트스트랩 내부에서 사용하는 구조이며, 애플리케이션은
// App.InterceptorFor를 통해 등록합니다.
type InterceptorBinding struct {
	Interceptor core.Interceptor
	Scope       configuration.InterceptorScope
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

func requiresShutdownSignal(config Config) bool {
	if config.HTTP != nil {
		return config.EnableGracefulShutdown
	}
	if len(config.CustomTransports) > 0 || len(config.ShutdownHooks) > 0 {
		return true
	}
	if config.ConsumerRegistry == nil || len(config.ConsumerRegistry.Registrations()) == 0 {
		return false
	}
	for _, binding := range config.EventAdapters {
		if !isNilHTTPValue(binding.Factory) && binding.Factory.Capabilities().Consume {
			return true
		}
	}
	return false
}

// Validate는 네트워크에 접근하지 않고 사전 검증을 수행하며,
// 사용자가 조치할 수 있는 모든 설정 문제를 하나의 오류로 반환합니다.
func Validate(config Config) error {
	_, err := validate(config)
	return err
}

// validate는 사전 검사에 사용한 UI 리소스를 준비 단계에도 전달합니다.
func validate(config Config) ([]configuration.OpenAPIResource, error) {
	issues := controllerProviderIssues(config)
	issues = append(issues, componentValidationIssues(config)...)
	issues = append(issues, shutdownHookIssues(config.ShutdownHooks)...)
	if config.ConsumerRegistry != nil {
		resolvers := consumerArgumentResolvers(config.Validator)
		for i, registration := range config.ConsumerRegistry.Registrations() {
			issues = append(issues, messageHandlerIssues(registration.Meta, resolvers, fmt.Sprintf("Consumers[%d]", i), "EVENT")...)
		}
	}
	if config.WebSocketRegistry != nil {
		resolvers := webSocketArgumentResolvers(config.Validator)
		for i, registration := range config.WebSocketRegistry.Registrations() {
			issues = append(issues, messageHandlerIssues(registration.Meta, resolvers, fmt.Sprintf("WebSockets[%d]", i), "WEBSOCKET")...)
		}
	}
	var openAPIResources []configuration.OpenAPIResource
	if config.ShutdownTimeout < 0 {
		issues = append(issues, configuration.ConfigIssue{
			Path:    "ShutdownTimeout",
			Code:    "SHUTDOWN_TIMEOUT_INVALID",
			Message: "Shutdown timeout cannot be negative.",
			Hint:    "Use zero for the default or a positive duration.",
		})
	}
	names := make(map[string]bool)
	var adapterErrors []error
	for i, binding := range config.EventAdapters {
		path := fmt.Sprintf("EventAdapters[%d]", i)
		if strings.TrimSpace(binding.Name) == "" || names[binding.Name] {
			issues = append(issues, configuration.ConfigIssue{Path: path + ".Name", Code: "EVENT_ADAPTER_NAME_INVALID", Message: "Event adapter names must be non-empty and unique.", Hint: "Assign a distinct name to each platform."})
		}
		names[binding.Name] = true
		issues = append(issues, configuration.ValidateConsumerRetry(binding.Options.ConsumerRetry, path+".ConsumerRetry")...)
		issues = append(issues, configuration.ValidateCleanupHistory(binding.Options.PublisherErrorHistory, path+".PublisherErrorHistory")...)
		issues = append(issues, configuration.ValidateCleanupHistory(binding.Options.ConsumerErrorHistory, path+".ConsumerErrorHistory")...)
		if binding.Options.SettlementTimeout < 0 {
			issues = append(issues, configuration.ConfigIssue{Path: path + ".SettlementTimeout", Code: "EVENT_SETTLEMENT_TIMEOUT_INVALID", Message: "Settlement timeout cannot be negative.", Hint: "Use zero for the default or a positive duration."})
		}
		if err := eventadapter.ValidateFactory(binding.Factory, eventadapter.Requirements{}); err != nil {
			adapterErrors = append(adapterErrors, fmt.Errorf("%s (%s): %w", path, binding.Name, err))
		}
	}

	if config.HTTP == nil {
		if config.WebSocketRegistry != nil && len(config.WebSocketRegistry.Registrations()) > 0 {
			issues = append(issues, configuration.ConfigIssue{
				Path: "HTTP", Code: "WEBSOCKET_HTTP_REQUIRED",
				Message: "WebSocket routes require the HTTP runtime.",
				Hint:    "Configure boot.Options.HTTP before registering WebSocket routes.",
			})
		}
	} else {
		prefix := config.HTTP.GlobalPrefix
		if prefix != "" && (!strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, ":*")) {
			issues = append(issues, configuration.ConfigIssue{
				Path: "HTTP.GlobalPrefix", Code: "HTTP_GLOBAL_PREFIX_INVALID",
				Message: "HTTP global prefix must start with '/' and cannot contain parameters or wildcards.",
				Hint:    "Use a static prefix such as /api/v1.",
			})
		}
		issues = append(issues, configuration.ValidateHTTP(*config.HTTP, "HTTP")...)
		resources, resourceIssues := openAPIUIResources(config.HTTP.OpenAPI)
		openAPIResources = resources
		issues = append(issues, resourceIssues...)
		issues = append(issues, openAPICollisionIssues(config, resources)...)
		issues = append(issues, httpWebSocketCollisionIssues(config)...)
	}
	for i, route := range config.Routes {
		if meta, err := spineRouter.Meta(route); err == nil {
			issues = append(issues, endpoint.HandlerDTOParameterIssues(meta, fmt.Sprintf("Routes[%d]", i), route.Path)...)
			issues = append(issues, endpoint.ResponseTypeIssues(meta, fmt.Sprintf("Routes[%d]", i))...)
		}
		if len(route.WebSocketInterceptors) != 0 || len(route.WebSocketUpgradeInterceptors) != 0 {
			issues = append(issues, configuration.ConfigIssue{Path: fmt.Sprintf("Routes[%d]", i), Code: "INTERCEPTOR_TRANSPORT_INVALID", Message: "HTTP routes use api.Interceptors."})
		}
		if _, err := spineRouter.Meta(route); err != nil {
			issues = append(issues, configuration.ConfigIssue{
				Path:    fmt.Sprintf("Routes[%d].Handler", i),
				Code:    "HTTP_ROUTE_HANDLER_INVALID",
				Message: fmt.Sprintf("HTTP route handler is invalid: %v", err),
				Hint:    "Provide a non-nil controller method expression such as (*Controller).Handle.",
			})
		}
	}

	bindings := make([]InterceptorBinding, 0, len(config.Interceptors)+len(config.ScopedInterceptors))
	for _, interceptor := range config.Interceptors {
		bindings = append(bindings, InterceptorBinding{Interceptor: interceptor, Scope: configuration.InterceptorAll})
	}
	bindings = append(bindings, config.ScopedInterceptors...)
	for i, binding := range bindings {
		path := fmt.Sprintf("Interceptors[%d]", i)
		if reflect.TypeOf(binding.Interceptor) == nil {
			issues = append(issues, configuration.ConfigIssue{Path: path, Code: "INTERCEPTOR_NIL", Message: "Global interceptor is nil.", Hint: "Remove the nil entry or provide an interceptor instance."})
		}
		if binding.Scope == 0 || binding.Scope&^configuration.InterceptorAll != 0 {
			issues = append(issues, configuration.ConfigIssue{Path: path + ".Scope", Code: "INTERCEPTOR_SCOPE_INVALID", Message: "Interceptor transport scope is invalid.", Hint: "Use boot.InterceptorHTTP for HTTP and app.WebSocketInterceptor for WebSocket."})
		}
	}
	issues = append(issues, webSocketInterceptorIssues(config)...)
	if err := configuration.ValidationError(issues...); err != nil {
		return nil, err
	}
	if config.ValidatorConfigured || config.Validator != nil {
		if err := validation.CheckValidator(config.Validator); err != nil {
			return nil, err
		}
	}
	for i, route := range config.Routes {
		meta, _ := spineRouter.Meta(route)
		issues = append(issues, endpoint.HandlerValidationParameterIssues(meta, fmt.Sprintf("Routes[%d]", i), config.Validator)...)
	}
	if err := configuration.ValidationError(issues...); err != nil {
		return nil, err
	}
	if config.HTTP != nil {
		hasWebSocketRoutes := config.WebSocketRegistry != nil && len(config.WebSocketRegistry.Registrations()) > 0
		if err := adapter.ValidateFactory(config.HTTPAdapterFactory, adapter.Requirements{TLS: config.HTTP.TLS != nil, NetHTTPUpgrade: hasWebSocketRoutes}); err != nil {
			adapterErrors = append(adapterErrors, err)
		} else if err := (adapter.Config{Options: serverOptions(*config.HTTP), Serve: adapter.ServeOptions{TLS: config.HTTP.TLS}, Execute: func(adapter.HTTPContext) error { return nil }}).Validate(config.HTTPAdapterFactory.Capabilities()); err != nil {
			adapterErrors = append(adapterErrors, err)
		}
		if hasWebSocketRoutes {
			if err := wsadapter.ValidateFactory(config.WebSocketAdapterFactory); err != nil {
				adapterErrors = append(adapterErrors, err)
			}
		}
	}
	return openAPIResources, errors.Join(adapterErrors...)
}

func serverOptions(options configuration.HTTPOptions) adapter.ServerOptions {
	if options.MaxBodyBytes == 0 {
		options.MaxBodyBytes = 32 << 20
	}
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

func webSocketInterceptorIssues(config Config) []configuration.ConfigIssue {
	var issues []configuration.ConfigIssue
	hasRoutes := config.WebSocketRegistry != nil && len(config.WebSocketRegistry.Registrations()) != 0
	for i, binding := range config.ScopedInterceptors {
		if binding.Scope == configuration.InterceptorWebSocket || (hasRoutes && binding.Scope == configuration.InterceptorAll) {
			issues = append(issues, configuration.ConfigIssue{
				Path:    fmt.Sprintf("Interceptors[%d].Scope", i),
				Code:    "WEBSOCKET_INTERCEPTOR_REGISTRATION_REQUIRED",
				Message: "WebSocket hooks require app.WebSocketInterceptor. Use app.InterceptorFor(boot.InterceptorHTTP, ...) for HTTP hooks in a mixed app.",
			})
		}
	}
	if hasRoutes && len(config.Interceptors) != 0 {
		issues = append(issues, configuration.ConfigIssue{Path: "Interceptors", Code: "WEBSOCKET_INTERCEPTOR_REGISTRATION_REQUIRED", Message: "WebSocket hooks require app.WebSocketInterceptor."})
	}
	for i, interceptor := range config.WebSocketInterceptors {
		if interceptor == nil {
			issues = append(issues, configuration.ConfigIssue{Path: fmt.Sprintf("WebSocketInterceptors[%d]", i), Code: "INTERCEPTOR_NIL", Message: "WebSocket interceptor is nil."})
		}
	}
	for i, interceptor := range config.WebSocketUpgradeInterceptors {
		if interceptor == nil {
			issues = append(issues, configuration.ConfigIssue{Path: fmt.Sprintf("WebSocketUpgradeInterceptors[%d]", i), Code: "INTERCEPTOR_NIL", Message: "WebSocket upgrade interceptor is nil."})
		}
	}
	return issues
}

func resolveGlobalInterceptors(ctr *container.Container, config Config) ([]core.Interceptor, error) {
	bindings := make([]core.Interceptor, 0, len(config.Interceptors)+len(config.ScopedInterceptors))
	bindings = append(bindings, config.Interceptors...)
	for _, binding := range config.ScopedInterceptors {
		if binding.Scope&configuration.InterceptorHTTP != 0 {
			bindings = append(bindings, binding.Interceptor)
		}
	}
	return resolveInterceptors(ctr, bindings, true)
}

// resolveInterceptors resolves typed-nil registrations without changing the registered order.
func resolveInterceptors[T any](ctr *container.Container, interceptors []T, deduplicate bool) ([]T, error) {
	resolved := make([]T, 0, len(interceptors))
	seen := make(map[interceptorIdentity]bool)
	for i, interceptor := range interceptors {
		t := reflect.TypeOf(interceptor)
		if t == nil {
			return nil, fmt.Errorf("interceptor[%d] is nil", i)
		}
		if deduplicate {
			if identity, identifiable := globalInterceptorIdentity(interceptor); identifiable {
				if seen[identity] {
					continue
				}
				seen[identity] = true
			}
		}
		if t.Kind() == reflect.Pointer && reflect.ValueOf(interceptor).IsNil() {
			instance, err := ctr.Resolve(t)
			if err != nil {
				return nil, fmt.Errorf("resolve interceptor[%d]: %w", i, err)
			}
			interceptor = instance.(T)
		}
		resolved = append(resolved, interceptor)
	}
	return resolved, nil
}

// rejectLegacyHandshakeHook prevents a previously active auth hook from being skipped.
func rejectLegacyHandshakeHook(interceptors []core.WebSocketInterceptor) error {
	for _, interceptor := range interceptors {
		if typ := reflect.TypeOf(interceptor); typ != nil {
			if _, exists := typ.MethodByName("PreHandshake"); exists {
				return fmt.Errorf("WEBSOCKET_HANDSHAKE_SIGNATURE_UNSUPPORTED: %s must migrate PreHandshake to BeforeUpgrade and register it with api.WebSocketUpgradeInterceptors or app.WebSocketUpgradeInterceptor", typ)
			}
		}
	}
	return nil
}

// globalInterceptorIdentity는 Go에서 실제 인스턴스의 정체성을 보존할 수 있을 때만
// 식별 정보를 반환합니다. 값이 같은 비포인터 인터셉터도 별도 등록일 수 있으므로
// 의도적으로 중복 제거하지 않습니다.
func globalInterceptorIdentity(interceptor any) (interceptorIdentity, bool) {
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
	return endpoint.Path(prefix, path)
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
	spineVersion = "v0.7.6"
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
	log.Printf("[Bootstrap] Spine version: %s", runtimeSpineVersion())
}

func buildConsumerPipeline(container *container.Container, validator validation.Validator) *pipeline.Pipeline {
	consumerPipeline := pipeline.NewPipeline(nil, invoker.NewInvoker(container))

	consumerPipeline.AddArgumentResolver(consumerArgumentResolvers(validator)...)
	return consumerPipeline
}

func consumerArgumentResolvers(validator validation.Validator) []resolver.ArgumentResolver {
	return []resolver.ArgumentResolver{
		&resolver.StdContextResolver{},
		&resolver.ControllerContextResolver{RequireExecutionContext: true},
		&eventResolver.EventNameResolver{},
		&eventResolver.PayloadResolver{},
		&eventResolver.DTOResolver{Validator: validator},
	}
}

func webSocketArgumentResolvers(v validation.Validator) []wsResolver.ArgumentResolver {
	return wsResolver.Resolvers(v)
}

type parameterSupporter interface {
	Supports(resolver.ParameterMeta) bool
}

func messageHandlerIssues[R parameterSupporter](meta execution.HandlerMeta, resolvers []R, prefix, transport string) []configuration.ConfigIssue {
	var issues []configuration.ConfigIssue
	signature := meta.FunctionType()
	if signature.IsVariadic() {
		issues = append(issues, configuration.ConfigIssue{
			Path: prefix, Code: transport + "_PARAMETER_UNSUPPORTED",
			Message: "Variadic message handlers are unsupported.",
			Hint:    "Declare a fixed parameter list.",
		})
	}
	for i := 0; i < signature.NumOut(); i++ {
		if result := signature.Out(i); !result.Implements(reflect.TypeFor[error]()) {
			issues = append(issues, configuration.ConfigIssue{
				Path: fmt.Sprintf("%s.Returns[%d]", prefix, i), Code: transport + "_RETURN_UNSUPPORTED",
				Message: fmt.Sprintf("No message return handler supports %s.", result),
				Hint:    "Return an error or use an explicit send or publish API.",
			})
		}
	}

	for _, parameter := range resolver.HandlerParameters(meta, nil) {
		if transport == "WEBSOCKET" && meta.Callback.IsValid() {
			payload, wrapped := wsResolver.MessagePayload(parameter.Type)
			valid := parameter.Type == reflect.TypeFor[context.Context]() || parameter.Type == reflect.TypeFor[publicWS.MessageContext]() || parameter.Type == reflect.TypeFor[publicWS.ConnectionID]()
			if wrapped {
				valid = payload.Kind() == reflect.Struct || payload == reflect.TypeFor[[]byte]()
			}
			if !valid {
				issues = append(issues, configuration.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "WEBSOCKET_PARAMETER_UNSUPPORTED", Message: "WebSocket callbacks require context.Context, ws.MessageContext, ws.ConnectionID or input.Message[struct or []byte]."})
				continue
			}
		}

		if !slices.ContainsFunc(resolvers, func(candidate R) bool {
			return candidate.Supports(parameter)
		}) {
			issues = append(issues, configuration.ConfigIssue{
				Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: transport + "_PARAMETER_UNSUPPORTED",
				Message: fmt.Sprintf("No resolver supports %s.", parameter.Type),
				Hint:    "Use a supported context, message identifier, raw bytes, or struct payload.",
			})
		}
	}
	return issues
}
