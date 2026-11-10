package core

import (
	"context"
	"mime/multipart"
)

type ContextCarrier interface {
	Context() context.Context
}

type EventBusCarrier interface {
	EventBus() EventBus
}

/*
ExecutionContext
- 파이프라인과 라우터 전용
- HTTP, WebSocket과 컨슈머의 공통 실행 흐름에서 사용
*/
type ExecutionContext interface {
	ContextCarrier
	EventBusCarrier

	Method() string
	Path() string
	Params() map[string]string
	Header(name string) string
	PathKeys() []string
	// SetRoute는 라우터가 찾은 요청별 파라미터와 키 순서를 저장합니다.
	// 구현체는 입력을 복사하여 사용자 저장소 및 다른 요청과 분리합니다.
	SetRoute(params map[string]string, pathKeys []string)
	Queries() map[string][]string
	Set(key string, value any)
	Get(key string) (any, bool)
}

// ControllerContext는 실행 파이프라인에서 사용하는 읽기 전용 컨텍스트 계약입니다.
// 애플리케이션 핸들러에서 인터셉터 값을 읽을 때는 pkg/spine의 Ctx를 사용하세요.
type ControllerContext interface {
	Get(key string) (any, bool)
}

/*
HttpRequestContext
- HTTP 전용 컨텍스트 계약
*/
type HttpRequestContext interface {
	ContextCarrier
	EventBusCarrier

	// 개별 접근
	Param(name string) string
	Query(name string) string
	Header(name string) string

	// 전체 뷰 접근
	Params() map[string]string
	Queries() map[string][]string
	Headers() map[string][]string

	// 요청 본문
	Bind(out any) error

	// 멀티파트
	MultipartForm() (*multipart.Form, error)
}

// HTTPResponseContext는 HTTP 응답 writer에 접근하는 실행 전용 계약입니다.
// 파이프라인은 응답 staging 동안 writer를 교체하고 전송 전에 원래 writer로 복구합니다.
// WS와 컨슈머 실행 컨텍스트에는 이 계약을 요구하지 않습니다.
type HTTPResponseContext interface {
	ResponseWriter() ResponseWriter
	SetResponseWriter(ResponseWriter)
}

/*
ConsumerRequestContext
- 이벤트 컨슈머 전용 컨텍스트
*/
type ConsumerRequestContext interface {
	ContextCarrier
	EventBusCarrier

	EventName() string
	Payload() []byte
}

/*
WebSocketContext
- WebSocket 전용 ExecutionContext 확장
*/
type WebSocketContext interface {
	ExecutionContext

	ConnID() string
	MessageType() int
	Payload() []byte
}

/*
WebSocketRequestContext
- WebSocket upgrade 요청에서 보존한 불변 요청 정보 뷰
- WebSocketContext와 별도인 추가 계약이므로 기존 사용자 구현을 깨지 않습니다.
*/
type WebSocketRequestContext interface {
	ContextCarrier

	Path() string
	Header(name string) string
	Headers() map[string][]string
	Query(name string) string
	Queries() map[string][]string
	Cookie(name string) (string, bool)
	Cookies() map[string]string
	RemoteAddr() string
	Host() string
	RequestURI() string
}

// WebSocketHandshakeContext는 HTTP upgrade 전에 인증/인가에 사용할 요청 뷰입니다.
type WebSocketHandshakeContext interface {
	WebSocketRequestContext
}

// WebSocketMessageContext는 메시지 처리와 원래 handshake 요청 정보를 함께 제공합니다.
type WebSocketMessageContext interface {
	WebSocketContext
	WebSocketRequestContext
}
