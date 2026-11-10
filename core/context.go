package core

import (
	"context"
	"mime/multipart"
)

type ContextCarrier interface {
	Context() context.Context
}

// ExecutionContext는 앱 인터셉터가 요청 정보와 실행별 저장소에 접근하는 계약입니다.
type ExecutionContext interface {
	ContextCarrier
	Method() string
	Path() string
	Params() map[string]string
	Header(name string) string
	PathKeys() []string
	Queries() map[string][]string
	Set(key string, value any)
	Get(key string) (any, bool)
}

// HTTPRequest는 HTTP 인터셉터가 요청과 동적 멀티파트 필드를 조회하는 계약입니다.
type HTTPRequest interface {
	ExecutionContext
	Param(name string) string
	Query(name string) string
	Headers() map[string][]string
	MultipartForm() (MultipartFields, error)
}

// MultipartFields는 HTTP 인터셉터가 조회한 현재 요청의 폼 필드 사본입니다.
// 파일은 현재 요청이 완료될 때까지 열 수 있습니다.
type MultipartFields struct {
	Value map[string][]string
	File  map[string][]*multipart.FileHeader
}

// HTTPResponseContext는 HTTP 인터셉터에 현재 응답 writer를 제공합니다.
type HTTPResponseContext interface {
	ResponseWriter() ResponseWriter
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
	ExecutionContext
	WebSocketRequestContext
	ConnID() string
	MessageType() int
	// Payload는 메시지 바이트의 독립 사본을 반환합니다.
	Payload() []byte
}
