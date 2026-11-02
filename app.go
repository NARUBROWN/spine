package spine

import (
	"context"
	"strings"

	adapter "github.com/NARUBROWN/spine/adapter"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/bootstrap"
	"github.com/NARUBROWN/spine/internal/event/consumer"
	"github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/internal/ws"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type App interface {
	// 생성자 선언
	Constructor(constructors ...any)
	// 라우트 선언
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
	constructors       []any
	routes             []router.RouteSpec
	interceptors       []bootstrap.InterceptorBinding
	customTransports   []core.CustomTransport
	consumerRegistry   *consumer.Registry
	websocketRegistry  *ws.Registry
	httpAdapterFactory adapter.Factory
}

// Option은 애플리케이션의 통합 구현을 명시적으로 선택하는 생성 옵션입니다.
type Option func(*app)

// WithHTTPAdapter는 HTTP 실행에 사용할 독립 계약의 factory를 지정합니다.
func WithHTTPAdapter(factory adapter.Factory) Option {
	return func(a *app) { a.httpAdapterFactory = factory }
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
	a.constructors = append(a.constructors, constructors...)
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
		Address:                opts.Address,
		Constructors:           a.constructors,
		Routes:                 a.routes,
		ScopedInterceptors:     a.interceptors,
		CustomTransports:       a.customTransports,
		EnableGracefulShutdown: opts.EnableGracefulShutdown,
		ShutdownTimeout:        opts.ShutdownTimeout,
		Kafka:                  opts.Kafka,
		RabbitMQ:               opts.RabbitMQ,
		ConsumerRegistry:       a.consumerRegistry,
		WebSocketRegistry:      a.websocketRegistry,
		HTTP:                   opts.HTTP,
		HTTPAdapterFactory:     a.httpAdapterFactory,
	}
}

func (a *app) Validate(opts boot.Options) error {
	return bootstrap.Validate(a.bootstrapConfig(opts))
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
