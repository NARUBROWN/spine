package contextview

import (
	"context"
	"mime/multipart"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
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
	HTTP() (HTTPView, bool)
	WebSocket() (WebSocketView, bool)
	// Deprecated: ExecutionContext().Get을 사용하세요.
	Get(key string) (any, bool)
}

type HTTPView interface {
	Request() HTTPRequestView
	Response() httpcontract.ResponseWriter
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
