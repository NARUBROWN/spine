package core

import (
	"context"
	"mime/multipart"
)

/*
RequestContext
- Resolver 공통 최소 계약
- HTTP / Consumer / gRPC 공통
*/
type RequestContext interface {
	ContextCarrier
	EventBusCarrier
}

type ContextCarrier interface {
	Context() context.Context
}

type EventBusCarrier interface {
	EventBus() EventBus
}

/*
ExecutionContext
- Pipeline / Router 전용
- HTTP Transport 실행 흐름에서만 사용
*/
type ExecutionContext interface {
	ContextCarrier
	EventBusCarrier

	Method() string
	Path() string
	Params() map[string]string
	Header(name string) string
	PathKeys() []string
	Queries() map[string][]string
	Set(key string, value any)
	Get(key string) (any, bool)
}

/*
ControllerContext
- Controller 전용 Context View
- ExecutionContext의 읽기 전용 Facade
- Interceptor에서 주입한 값을 Controller에서 참조하기 위한 공식 통로
*/
type ControllerContext interface {
	Get(key string) (any, bool)
}

/*
HttpRequestContext
- HTTP 전용 RequestContext 확장
*/
type HttpRequestContext interface {
	RequestContext

	// 개별 접근
	Param(name string) string
	Query(name string) string
	Header(name string) string

	// 전체 뷰 접근
	Params() map[string]string
	Queries() map[string][]string
	Headers() map[string][]string

	// body
	Bind(out any) error

	// Multipart
	MultipartForm() (*multipart.Form, error)
}

/*
ConsumerRequestContext
- Event Consumer 전용 Context
*/
type ConsumerRequestContext interface {
	RequestContext

	EventName() string
	Payload() []byte
}
