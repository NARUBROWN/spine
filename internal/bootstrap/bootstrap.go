package bootstrap

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	eventadapter "github.com/NARUBROWN/spine/contract/event"
	adapter "github.com/NARUBROWN/spine/contract/http"
	validation "github.com/NARUBROWN/spine/contract/validator"
	wsadapter "github.com/NARUBROWN/spine/contract/websocket"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/event/consumer"
	eventResolver "github.com/NARUBROWN/spine/internal/event/consumer/resolver"
	"github.com/NARUBROWN/spine/internal/event/hook"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/NARUBROWN/spine/internal/resolver"
	spineRouter "github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/internal/ws"
	wsResolver "github.com/NARUBROWN/spine/internal/ws/resolver"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type Config struct {
	Address                 string
	Constructors            []any
	Routes                  []spineRouter.RouteSpec
	Interceptors            []core.Interceptor
	ScopedInterceptors      []InterceptorBinding
	CustomTransports        []core.CustomTransport
	EnableGracefulShutdown  bool
	ShutdownTimeout         time.Duration
	EventAdapters           []EventAdapterBinding
	ConsumerRegistry        *consumer.Registry
	HTTP                    *boot.HTTPOptions
	HTTPAdapterFactory      adapter.Factory
	WebSocketAdapterFactory wsadapter.Factory
	WebSocketRegistry       *ws.Registry
	Validator               validation.Validator
	ValidatorConfigured     bool
}

// EventAdapterBinding은 구현과 공통 실행 정책을 연결하는 준비 설정입니다.
type EventAdapterBinding struct {
	Name    string
	Factory eventadapter.Factory
	Options boot.EventOptions
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
	var issues []boot.ConfigIssue
	if config.ShutdownTimeout < 0 {
		issues = append(issues, boot.ConfigIssue{
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
			issues = append(issues, boot.ConfigIssue{Path: path + ".Name", Code: "EVENT_ADAPTER_NAME_INVALID", Message: "Event adapter names must be non-empty and unique.", Hint: "Assign a distinct name to each platform."})
		}
		names[binding.Name] = true
		issues = append(issues, binding.Options.ConsumerRetry.ValidateIssues(path+".ConsumerRetry")...)
		if binding.Options.SettlementTimeout < 0 {
			issues = append(issues, boot.ConfigIssue{Path: path + ".SettlementTimeout", Code: "EVENT_SETTLEMENT_TIMEOUT_INVALID", Message: "Settlement timeout cannot be negative.", Hint: "Use zero for the default or a positive duration."})
		}
		if err := eventadapter.ValidateFactory(binding.Factory, eventadapter.Requirements{}); err != nil {
			adapterErrors = append(adapterErrors, fmt.Errorf("%s (%s): %w", path, binding.Name, err))
		}
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
	if config.ValidatorConfigured || config.Validator != nil {
		if err := validation.CheckValidator(config.Validator); err != nil {
			return err
		}
	}
	if config.HTTP != nil {
		hasWebSocketRoutes := config.WebSocketRegistry != nil && len(config.WebSocketRegistry.Registrations()) > 0
		if err := adapter.ValidateFactory(config.HTTPAdapterFactory, adapter.Requirements{WebSocketUpgrade: hasWebSocketRoutes}); err != nil {
			adapterErrors = append(adapterErrors, err)
		}
		if hasWebSocketRoutes {
			if err := wsadapter.ValidateFactory(config.WebSocketAdapterFactory); err != nil {
				adapterErrors = append(adapterErrors, err)
			}
		}
	}
	return errors.Join(adapterErrors...)
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
	spineVersion = "v0.6.5"
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

func buildConsumerPipeline(container *container.Container, registry *consumer.Registry, dispatchHook *hook.EventDispatchHook, validator validation.Validator) *pipeline.Pipeline {
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
		&eventResolver.DTOResolver{Validator: validator},
	)

	return consumerPipeline
}

func buildWSPipeline(
	container *container.Container,
	registry *ws.Registry,
	dispatchHook *hook.EventDispatchHook,
	interceptors []core.Interceptor,
	validator validation.Validator,
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
		&wsResolver.DTOResolver{Validator: validator},
	)

	return wsPipeline
}
