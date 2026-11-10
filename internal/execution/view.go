package execution

import (
	"context"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"slices"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/internal/contextview"
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
func (v publicContext) HTTP() (contextview.HTTPView, bool) {
	request, ok := v.source.(HTTPRequestContext)
	if !ok {
		return nil, false
	}
	response, ok := v.source.(HTTPResponseContext)
	if !ok {
		return nil, false
	}
	return httpView{
		source:   v.source,
		request:  request,
		response: response,
	}, true
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

type httpView struct {
	source   Context
	request  HTTPRequestContext
	response HTTPResponseContext
}

func (v httpView) Context() context.Context              { return GoContext(v.source) }
func (v httpView) Request() contextview.HTTPRequestView  { return v }
func (v httpView) Response() httpcontract.ResponseWriter { return v.response.ResponseWriter() }
func (v httpView) Method() string                        { return v.source.Method() }
func (v httpView) Path() string                          { return v.source.Path() }
func (v httpView) Params() map[string]string             { return v.source.Params() }
func (v httpView) PathKeys() []string                    { return v.source.PathKeys() }
func (v httpView) Header(name string) string             { return v.request.Header(name) }
func (v httpView) Queries() map[string][]string          { return v.request.Queries() }

func (v httpView) Param(name string) string     { return v.request.Param(name) }
func (v httpView) Query(name string) string     { return v.request.Query(name) }
func (v httpView) Headers() map[string][]string { return v.request.Headers() }
func (v httpView) MultipartForm() (contextview.MultipartFields, error) {
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
			if file != nil {
				header := *file
				header.Header = textproto.MIMEHeader(http.Header(file.Header).Clone())
				headers[index] = &header
			}
		}
		fields.File[name] = headers
	}
	return fields, nil
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
