package execution

import (
	"context"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"slices"

	"github.com/NARUBROWN/spine/core"
)

// InterceptorView는 실행 객체를 그대로 넘기지 않고 전송별 공개 기능을 제공합니다.
func InterceptorView(ctx Context) core.ExecutionContext {
	view := interceptorView{source: ctx}
	if message, ok := ctx.(WebSocketContext); ok {
		if request, ok := ctx.(core.WebSocketRequestContext); ok {
			return webSocketView{interceptorView: view, request: request, message: message}
		}
	}
	if response, ok := ctx.(HTTPResponseContext); ok {
		responseView := httpResponseView{interceptorView: view, response: response}
		if request, ok := ctx.(HTTPRequestContext); ok {
			return httpView{httpResponseView: responseView, request: request}
		}
		return responseView
	}
	return view
}

type interceptorView struct{ source Context }

func (v interceptorView) Context() context.Context     { return GoContext(v.source) }
func (v interceptorView) Method() string               { return v.source.Method() }
func (v interceptorView) Path() string                 { return v.source.Path() }
func (v interceptorView) Params() map[string]string    { return v.source.Params() }
func (v interceptorView) Header(name string) string    { return v.source.Header(name) }
func (v interceptorView) PathKeys() []string           { return v.source.PathKeys() }
func (v interceptorView) Queries() map[string][]string { return v.source.Queries() }
func (v interceptorView) Set(key string, value any)    { v.source.Set(key, value) }
func (v interceptorView) Get(key string) (any, bool)   { return v.source.Get(key) }

type httpResponseView struct {
	interceptorView
	response HTTPResponseContext
}

func (v httpResponseView) ResponseWriter() core.ResponseWriter { return v.response.ResponseWriter() }

type httpView struct {
	httpResponseView
	request HTTPRequestContext
}

func (v httpView) Param(name string) string     { return v.request.Param(name) }
func (v httpView) Query(name string) string     { return v.request.Query(name) }
func (v httpView) Headers() map[string][]string { return v.request.Headers() }
func (v httpView) MultipartForm() (core.MultipartFields, error) {
	form, err := v.request.MultipartForm()
	if err != nil || form == nil {
		return core.MultipartFields{}, err
	}

	fields := core.MultipartFields{
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
	interceptorView
	request core.WebSocketRequestContext
	message WebSocketContext
}

func (v webSocketView) ConnID() string                    { return v.message.ConnID() }
func (v webSocketView) MessageType() int                  { return v.message.MessageType() }
func (v webSocketView) Payload() []byte                   { return slices.Clone(v.message.Payload()) }
func (v webSocketView) Headers() map[string][]string      { return v.request.Headers() }
func (v webSocketView) Query(name string) string          { return v.request.Query(name) }
func (v webSocketView) Cookie(name string) (string, bool) { return v.request.Cookie(name) }
func (v webSocketView) Cookies() map[string]string        { return v.request.Cookies() }
func (v webSocketView) RemoteAddr() string                { return v.request.RemoteAddr() }
func (v webSocketView) Host() string                      { return v.request.Host() }
func (v webSocketView) RequestURI() string                { return v.request.RequestURI() }
