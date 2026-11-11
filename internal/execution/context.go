package execution

import (
	"context"
	"mime/multipart"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/internal/event/emission"
)

// Context는 전송 상태와 실행 파이프라인의 제어를 함께 제공합니다.
type Context interface {
	Context() context.Context
	EventBus() emission.Bus
	Method() string
	Path() string
	Params() map[string]string
	Header(name string) string
	PathKeys() []string
	SetRoute(params map[string]string, pathKeys []string)
	Queries() map[string][]string
	Set(key string, value any)
	Get(key string) (any, bool)
}

type HTTPRequestContext interface {
	Context() context.Context
	Method() string
	Path() string
	PathKeys() []string
	Param(name string) string
	Query(name string) string
	Header(name string) string
	Params() map[string]string
	Queries() map[string][]string
	Headers() map[string][]string
	Bind(out any, allowedMedia ...[]string) error
	MultipartForm() (*multipart.Form, error)
}

type HTTPResponseContext interface {
	ResponseWriter() httpcontract.ResponseWriter
	SetResponseWriter(httpcontract.ResponseWriter)
}

type ConsumerRequestContext interface {
	EventName() string
	Payload() []byte
}

type WebSocketContext interface {
	Context
	ConnID() string
	MessageType() int
	Payload() []byte
}

func GoContext(ctx Context) context.Context {
	parent := ctx.Context()
	if bus := ctx.EventBus(); bus != nil {
		return emission.WithPublisher(parent, bus)
	}
	return parent
}

// WebSocketRequestContext는 내부 upgrade 요청의 불변 정보 계약입니다.
type WebSocketRequestContext interface {
	Context() context.Context
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
