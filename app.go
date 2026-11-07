package spine

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/NARUBROWN/spine/contract/description"
	eventadapter "github.com/NARUBROWN/spine/contract/event"
	adapter "github.com/NARUBROWN/spine/contract/http"
	validation "github.com/NARUBROWN/spine/contract/validator"
	wsadapter "github.com/NARUBROWN/spine/contract/websocket"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/bootstrap"
	"github.com/NARUBROWN/spine/internal/event/consumer"
	"github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/internal/ws"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
)

type App interface {
	// Constructor는 constructor의 역할 표식 또는 기존 생성자 함수를 등록합니다.
	Constructor(constructors ...any)
	// Route는 메서드 표현식으로 HTTP 경로를 등록합니다.
	//
	// Deprecated: 새 HTTP 경로는 api.Controller의 RegisterRoutes 메서드에 등록하세요.
	// 기존 등록은 전환 기간에 계속 사용할 수 있습니다.
	Route(method string, path string, handler any, opts ...router.RouteOption)
	// 인터셉터 선언
	Interceptor(interceptors ...core.Interceptor)
	// InterceptorFor는 선택한 기본 전송 방식에 전역 인터셉터를 등록합니다.
	InterceptorFor(scope boot.InterceptorScope, interceptors ...core.Interceptor)
	// 독립 실행되는 사용자 정의 전송 방식 등록
	RegisterTransport(t core.CustomTransport)
	// Validate는 네트워크 연결을 열지 않고 애플리케이션 설정을 검사합니다.
	Validate(opts boot.Options) error
	// Prepare는 리스너 없이 앱을 준비합니다. 반환한 결과는 호출자가 Close해야 합니다.
	Prepare(ctx context.Context, opts boot.Options) (PreparedApplication, error)
	// 실행
	Run(opts boot.Options) error
	// RunContext는 context가 취소될 때 정상 종료를 시작합니다.
	RunContext(ctx context.Context, opts boot.Options) error
	// 이벤트 소비자 레지스트리 반환
	Consumers() *consumer.Registry
	// 웹 소켓 레지스트리 반환
	WebSocket() *ws.Registry
}

type app struct {
	constructors            []any
	controllerConstructors  []any
	componentIssues         []boot.ConfigIssue
	componentCounts         map[string]int
	typedComponents         bool
	routes                  []router.RouteSpec
	interceptors            []bootstrap.InterceptorBinding
	customTransports        []core.CustomTransport
	consumerRegistry        *consumer.Registry
	websocketRegistry       *ws.Registry
	httpAdapterFactory      adapter.Factory
	websocketAdapterFactory wsadapter.Factory
	eventAdapters           []bootstrap.EventAdapterBinding
	validator               validation.Validator
	validatorConfigured     bool
}

// Option은 애플리케이션의 통합 구현을 명시적으로 선택하는 생성 옵션입니다.
type Option func(*app)

// WithHTTPAdapter는 HTTP 실행에 사용할 독립 계약의 factory를 지정합니다.
func WithHTTPAdapter(factory adapter.Factory) Option {
	return func(a *app) { a.httpAdapterFactory = factory }
}

// WithWebSocketAdapter는 HTTP 플랫폼과 독립적으로 WebSocket 구현을 선택합니다.
// WS 경로를 등록한 앱은 factory를 명시해야 합니다.
func WithWebSocketAdapter(factory wsadapter.Factory) Option {
	return func(a *app) { a.websocketAdapterFactory = factory }
}

// EventAdapterOption은 브로커 공통 실행 정책을 설정합니다.
type EventAdapterOption func(*boot.EventOptions)

// WithEventAdapter는 이름으로 식별하는 이벤트 플랫폼 factory를 등록합니다.
// 읽기와 발행 역할은 factory의 불변 Capabilities 선언을 따릅니다.
func WithEventAdapter(name string, factory eventadapter.Factory, options ...EventAdapterOption) Option {
	settings := boot.EventOptions{}
	for _, option := range options {
		if option != nil {
			option(&settings)
		}
	}
	return func(a *app) {
		a.eventAdapters = append(a.eventAdapters, bootstrap.EventAdapterBinding{Name: name, Factory: factory, Options: settings})
	}
}

// WithEventConsumerRetry는 Reader 생성과 재연결의 공통 backoff를 설정합니다.
func WithEventConsumerRetry(policy boot.ConsumerRetryOptions) EventAdapterOption {
	return func(options *boot.EventOptions) { options.ConsumerRetry = policy }
}

// WithEventSettlementTimeout은 Ack/Nack 완료를 기다릴 최대 시간을 설정합니다.
func WithEventSettlementTimeout(timeout time.Duration) EventAdapterOption {
	return func(options *boot.EventOptions) { options.SettlementTimeout = timeout }
}

// WithEventPublisherErrorHistory는 발행기 정리 실패의 보관 건수와 기간을 설정합니다.
func WithEventPublisherErrorHistory(policy boot.CleanupErrorHistoryOptions) EventAdapterOption {
	return func(options *boot.EventOptions) { options.PublisherErrorHistory = policy }
}

// WithEventConsumerErrorHistory는 Reader 정리 실패의 보관 건수와 기간을 설정합니다.
func WithEventConsumerErrorHistory(policy boot.CleanupErrorHistoryOptions) EventAdapterOption {
	return func(options *boot.EventOptions) { options.ConsumerErrorHistory = policy }
}

// WithValidator는 모든 기본 DTO 리졸버가 사용할 검증기를 등록합니다.
// 생략하면 검증하지 않으며, 명시적으로 등록한 nil은 앱 사전 검사에서 거부합니다.
func WithValidator(validator validation.Validator) Option {
	return func(a *app) {
		a.validator = validator
		a.validatorConfigured = true
	}
}

// New는 특정 HTTP 서버 구현을 선택하지 않는 Spine 커널 앱을 생성합니다.
// HTTP를 사용할 때는 통합 모듈의 생성자 또는 WithHTTPAdapter를 사용합니다.
// 예: New(WithHTTPAdapter(factory)). HTTP 없는 앱은 New()로 생성합니다.
func New(options ...Option) App {
	a := &app{}
	for _, option := range options {
		if option != nil {
			option(a)
		}
	}
	return a
}

func (a *app) Constructor(constructors ...any) {
	for _, value := range constructors {
		if registration, ok := value.(constructor.Registration); ok {
			a.componentRegistration(registration)
		} else if t := reflect.TypeOf(value); t != nil && t.Kind() == reflect.Func {
			a.constructors = append(a.constructors, value)
			if a.controllerConstructors != nil {
				a.controllerConstructors = append(a.controllerConstructors, value)
			}
		} else {
			a.componentConstructors("Custom", nil, []any{value})
		}
	}
}

func (a *app) Route(method string, path string, handler any, opts ...router.RouteOption) {
	// HTTP 메서드를 대문자로 변환해 라우팅 시 대소문자 불일치 문제를 방지합니다.
	method = strings.ToUpper(strings.TrimSpace(method))

	spec := router.RouteSpec{
		Method:  method,
		Path:    path,
		Handler: handler,
	}

	for _, opt := range opts {
		opt(&spec)
	}

	a.routes = append(a.routes, spec)
}

func (a *app) Interceptor(interceptors ...core.Interceptor) {
	a.InterceptorFor(boot.InterceptorAll, interceptors...)
}

func (a *app) InterceptorFor(scope boot.InterceptorScope, interceptors ...core.Interceptor) {
	for _, interceptor := range interceptors {
		a.interceptors = append(a.interceptors, bootstrap.InterceptorBinding{
			Interceptor: interceptor,
			Scope:       scope,
		})
	}
}

func (a *app) RegisterTransport(t core.CustomTransport) {
	a.customTransports = append(a.customTransports, t)
}

func (a *app) bootstrapConfig(opts boot.Options) bootstrap.Config {
	return bootstrap.Config{
		Address:                 opts.Address,
		Constructors:            a.constructors,
		ControllerConstructors:  a.controllerConstructors,
		ComponentIssues:         a.componentIssues,
		TypedComponents:         a.typedComponents,
		Routes:                  a.routes,
		ScopedInterceptors:      a.interceptors,
		CustomTransports:        a.customTransports,
		EnableGracefulShutdown:  opts.EnableGracefulShutdown,
		ShutdownTimeout:         opts.ShutdownTimeout,
		EventAdapters:           a.eventAdapters,
		ConsumerRegistry:        a.consumerRegistry,
		WebSocketRegistry:       a.websocketRegistry,
		HTTP:                    opts.HTTP,
		HTTPAdapterFactory:      a.httpAdapterFactory,
		WebSocketAdapterFactory: a.websocketAdapterFactory,
		Validator:               a.validator,
		ValidatorConfigured:     a.validatorConfigured,
	}
}

func (a *app) Validate(opts boot.Options) error {
	return bootstrap.Validate(a.bootstrapConfig(opts))
}

// HTTPDescriber는 기존 App 구현을 확장하지 않는 선택적 HTTP 설명 기능입니다.
type HTTPDescriber interface {
	DescribeHTTP(boot.Options) (description.HTTP, error)
}

// ErrHTTPDescriptionUnavailable는 선택적 HTTP 설명 기능이 없는 앱을 나타냅니다.
var ErrHTTPDescriptionUnavailable = errors.New("application does not provide HTTP descriptions")

// DescribeHTTP는 생성자나 네트워크를 실행하지 않고 현재 HTTP 등록을 설명합니다.
func DescribeHTTP(application App, opts boot.Options) (description.HTTP, error) {
	if describer, ok := application.(HTTPDescriber); ok {
		return describer.DescribeHTTP(opts)
	}
	return description.HTTP{}, ErrHTTPDescriptionUnavailable
}

func (a *app) DescribeHTTP(opts boot.Options) (description.HTTP, error) {
	return bootstrap.DescribeHTTP(a.bootstrapConfig(opts))
}

func (a *app) Run(opts boot.Options) error {
	return bootstrap.Run(a.bootstrapConfig(opts))
}

func (a *app) RunContext(ctx context.Context, opts boot.Options) error {
	return bootstrap.RunContext(ctx, a.bootstrapConfig(opts))
}

func (a *app) Consumers() *consumer.Registry {
	if a.consumerRegistry == nil {
		a.consumerRegistry = consumer.NewRegistry()
	}
	return a.consumerRegistry
}

func (a *app) WebSocket() *ws.Registry {
	if a.websocketRegistry == nil {
		a.websocketRegistry = ws.NewRegistry()
	}
	return a.websocketRegistry
}
