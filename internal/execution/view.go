package execution

import (
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"slices"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/internal/contextview"
	"github.com/NARUBROWN/spine/internal/input"
)

// NewContext는 내부 실행을 감싼 공통 공개 컨텍스트를 반환합니다.
func NewContext(ctx Context) contextview.Ctx {
	return publicContext{source: ctx}
}

type publicContext struct{ source Context }

func (v publicContext) Get(key string) (any, bool) { return v.source.Get(key) }
func (v publicContext) ExecutionContext() contextview.ExecutionContextView {
	return executionView{source: v.source}
}
func (v publicContext) HTTPRequestContext() (contextview.HTTPRequestView, bool) {
	request, ok := v.source.(HTTPRequestContext)
	if !ok {
		return nil, false
	}
	return httpRequestView{source: v.source, request: request}, true
}
func (v publicContext) HTTPResponseContext() (contextview.HTTPResponseView, bool) {
	response, ok := v.source.(HTTPResponseContext)
	if !ok || response.ResponseWriter() == nil {
		return nil, false
	}
	return httpResponseView{response: response}, true
}
func (v publicContext) WebSocket() (contextview.WebSocketView, bool) {
	message, ok := v.source.(WebSocketContext)
	if !ok {
		return nil, false
	}
	request, ok := v.source.(WebSocketRequestContext)
	if !ok {
		return nil, false
	}
	return webSocketView{
		source:  v.source,
		request: request,
		message: message,
	}, true
}

type executionView struct{ source Context }

func (v executionView) Context() context.Context   { return GoContext(v.source) }
func (v executionView) Get(key string) (any, bool) { return v.source.Get(key) }
func (v executionView) Set(key string, value any)  { v.source.Set(key, value) }

type httpRequestView struct {
	source  Context
	request HTTPRequestContext
}

func (v httpRequestView) Context() context.Context     { return GoContext(v.source) }
func (v httpRequestView) Method() string               { return v.request.Method() }
func (v httpRequestView) Path() string                 { return v.request.Path() }
func (v httpRequestView) Params() map[string]string    { return v.request.Params() }
func (v httpRequestView) PathKeys() []string           { return v.request.PathKeys() }
func (v httpRequestView) Header(name string) string    { return v.request.Header(name) }
func (v httpRequestView) Queries() map[string][]string { return v.request.Queries() }

func (v httpRequestView) Param(name string) string     { return v.request.Param(name) }
func (v httpRequestView) Query(name string) string     { return v.request.Query(name) }
func (v httpRequestView) Headers() map[string][]string { return v.request.Headers() }
func (v httpRequestView) MultipartForm() (contextview.MultipartFields, error) {
	form, err := v.request.MultipartForm()
	if err != nil || form == nil {
		return contextview.MultipartFields{}, err
	}

	fields := contextview.MultipartFields{
		Value: make(map[string][]string, len(form.Value)),
		File:  make(map[string][]*multipart.FileHeader, len(form.File)),
	}
	for name, values := range form.Value {
		fields.Value[name] = slices.Clone(values)
	}
	for name, files := range form.File {
		headers := make([]*multipart.FileHeader, len(files))
		for index, file := range files {
			headers[index] = input.CloneFileHeader(file)
		}
		fields.File[name] = headers
	}
	return fields, nil
}

// httpResponseView는 staging 변경 뒤에도 현재 writer에 위임합니다.
type httpResponseView struct{ response HTTPResponseContext }

var errResponseUnavailable = errors.New("HTTP response is no longer available")

func (v httpResponseView) useWriter(use func(httpcontract.ResponseWriter)) bool {
	if request, ok := v.response.(interface {
		UseResponseWriter(func(httpcontract.ResponseWriter)) bool
	}); ok {
		return request.UseResponseWriter(use)
	}
	writer := v.response.ResponseWriter()
	if writer == nil {
		return false
	}
	use(writer)
	return true
}

func (v httpResponseView) write(write func(httpcontract.ResponseWriter) error) (err error) {
	if !v.useWriter(func(writer httpcontract.ResponseWriter) { err = write(writer) }) {
		return errResponseUnavailable
	}
	return err
}

func (v httpResponseView) SetHeader(key, value string) {
	v.useWriter(func(writer httpcontract.ResponseWriter) { writer.SetHeader(key, value) })
}
func (v httpResponseView) AddHeader(key, value string) {
	v.useWriter(func(writer httpcontract.ResponseWriter) { writer.AddHeader(key, value) })
}
func (v httpResponseView) IsCommitted() (committed bool) {
	v.useWriter(func(writer httpcontract.ResponseWriter) { committed = writer.IsCommitted() })
	return committed
}
func (v httpResponseView) WriteStatus(status int) error {
	return v.write(func(writer httpcontract.ResponseWriter) error { return writer.WriteStatus(status) })
}
func (v httpResponseView) WriteJSON(status int, body json.RawMessage) error {
	return v.write(func(writer httpcontract.ResponseWriter) error { return writer.WriteJSON(status, body) })
}
func (v httpResponseView) WriteString(status int, body string) error {
	return v.write(func(writer httpcontract.ResponseWriter) error { return writer.WriteString(status, body) })
}
func (v httpResponseView) WriteBytes(status int, body []byte) error {
	return v.write(func(writer httpcontract.ResponseWriter) error { return writer.WriteBytes(status, body) })
}

type webSocketView struct {
	source  Context
	request WebSocketRequestContext
	message WebSocketContext
}

func (v webSocketView) Context() context.Context          { return GoContext(v.source) }
func (v webSocketView) ConnID() string                    { return v.message.ConnID() }
func (v webSocketView) MessageType() int                  { return v.message.MessageType() }
func (v webSocketView) Payload() []byte                   { return slices.Clone(v.message.Payload()) }
func (v webSocketView) Path() string                      { return v.request.Path() }
func (v webSocketView) Header(name string) string         { return v.request.Header(name) }
func (v webSocketView) Queries() map[string][]string      { return v.request.Queries() }
func (v webSocketView) Headers() map[string][]string      { return v.request.Headers() }
func (v webSocketView) Query(name string) string          { return v.request.Query(name) }
func (v webSocketView) Cookie(name string) (string, bool) { return v.request.Cookie(name) }
func (v webSocketView) Cookies() map[string]string        { return v.request.Cookies() }
func (v webSocketView) RemoteAddr() string                { return v.request.RemoteAddr() }
func (v webSocketView) Host() string                      { return v.request.Host() }
func (v webSocketView) RequestURI() string                { return v.request.RequestURI() }
