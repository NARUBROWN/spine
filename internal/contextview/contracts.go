package contextview

import (
	"context"
	"encoding/json"
	"mime/multipart"
)

type ContextCarrier interface {
	Context() context.Context
}

type ExecutionContextView = interface {
	ContextCarrier
	Get(key string) (any, bool)
	Set(key string, value any)
}

type Ctx interface {
	ExecutionContext() ExecutionContextView
	HTTPRequestContext() (HTTPRequestView, bool)
	HTTPResponseContext() (HTTPResponseView, bool)
	WebSocket() (WebSocketView, bool)
	// Deprecated: ExecutionContext().Get을 사용하세요.
	Get(key string) (any, bool)
}

// HTTPResponseView는 내부 writer 교체를 허용하지 않는 응답 출력 계약입니다.
type HTTPResponseView interface {
	SetHeader(key, value string)
	AddHeader(key, value string)
	IsCommitted() bool
	WriteStatus(status int) error
	WriteJSON(status int, body json.RawMessage) error
	WriteString(status int, body string) error
	WriteBytes(status int, body []byte) error
}

type HTTPRequestView interface {
	ContextCarrier
	Method() string
	Path() string
	Param(name string) string
	Params() map[string]string
	PathKeys() []string
	Query(name string) string
	Queries() map[string][]string
	Header(name string) string
	Headers() map[string][]string
	MultipartForm() (MultipartFields, error)
}

type MultipartFields struct {
	Value map[string][]string
	File  map[string][]*multipart.FileHeader
}

type WebSocketRequestView interface {
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

type WebSocketView interface {
	WebSocketRequestView
	ConnID() string
	MessageType() int
	Payload() []byte
}
