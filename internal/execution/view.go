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
func NewContext(ctx Context, input *Input) contextview.Ctx {
	return publicContext{source: ctx, input: input}
}

type publicContext struct {
	source Context
	input  *Input
}

func (v publicContext) Get(key string) (any, bool) { return v.source.Get(key) }
func (v publicContext) ExecutionContext() contextview.ExecutionContextView {
	return executionView{source: v.source}
}
func (v publicContext) HTTPRequestContext() (contextview.HTTPRequestView, bool) {
	if v.input.HTTPRequest() == nil {
		return nil, false
	}
	return httpRequestView{source: v.source, input: v.input}, true
}
func (v publicContext) HTTPResponseContext() (contextview.HTTPResponseView, bool) {
	available := false
	v.input.read(func(s Sources) { available = s.HTTPResponse != nil && s.HTTPResponse.ResponseWriter() != nil })
	if !available {
		return nil, false
	}
	return httpResponseView{input: v.input}, true
}

// readInput holds the lease while a view reads borrowed transport data.
func readInput[T any](i *Input, read func(Sources) T) (value T) {
	i.read(func(s Sources) { value = read(s) })
	return
}

type executionView struct{ source Context }

func (v executionView) Context() context.Context   { return v.source.Context() }
func (v executionView) Get(key string) (any, bool) { return v.source.Get(key) }
func (v executionView) Set(key string, value any)  { v.source.Set(key, value) }

type httpRequestView struct {
	source Context
	input  *Input
}

func (v httpRequestView) Context() context.Context { return v.source.Context() }
func (v httpRequestView) Method() string {
	return readInput(v.input, func(s Sources) string {
		if s.HTTPRequest == nil {
			var zero string
			return zero
		}
		return s.HTTPRequest.Method()
	})
}
func (v httpRequestView) Path() string {
	return readInput(v.input, func(s Sources) string {
		if s.HTTPRequest == nil {
			var zero string
			return zero
		}
		return s.HTTPRequest.Path()
	})
}
func (v httpRequestView) Params() map[string]string {
	return readInput(v.input, func(s Sources) map[string]string {
		if s.HTTPRequest == nil {
			var zero map[string]string
			return zero
		}
		return s.HTTPRequest.Params()
	})
}
func (v httpRequestView) PathKeys() []string {
	return readInput(v.input, func(s Sources) []string {
		if s.HTTPRequest == nil {
			var zero []string
			return zero
		}
		return s.HTTPRequest.PathKeys()
	})
}
func (v httpRequestView) Header(name string) string {
	return readInput(v.input, func(s Sources) string {
		if s.HTTPRequest == nil {
			var zero string
			return zero
		}
		return s.HTTPRequest.Header(name)
	})
}
func (v httpRequestView) Queries() map[string][]string {
	return readInput(v.input, func(s Sources) map[string][]string {
		if s.HTTPRequest == nil {
			var zero map[string][]string
			return zero
		}
		return s.HTTPRequest.Queries()
	})
}
func (v httpRequestView) Param(name string) string {
	return readInput(v.input, func(s Sources) string {
		if s.HTTPRequest == nil {
			var zero string
			return zero
		}
		return s.HTTPRequest.Param(name)
	})
}
func (v httpRequestView) Query(name string) string {
	return readInput(v.input, func(s Sources) string {
		if s.HTTPRequest == nil {
			var zero string
			return zero
		}
		return s.HTTPRequest.Query(name)
	})
}
func (v httpRequestView) Headers() map[string][]string {
	return readInput(v.input, func(s Sources) map[string][]string {
		if s.HTTPRequest == nil {
			var zero map[string][]string
			return zero
		}
		return s.HTTPRequest.Headers()
	})
}
func (v httpRequestView) MultipartForm() (fields contextview.MultipartFields, err error) {
	v.input.read(func(s Sources) {
		if s.HTTPRequest == nil {
			err = errResponseUnavailable
			return
		}
		fields, err = cloneMultipartFields(s.HTTPRequest)
	})
	return
}
func cloneMultipartFields(request HTTPRequestContext) (contextview.MultipartFields, error) {
	form, err := request.MultipartForm()
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
type httpResponseView struct{ input *Input }

var errResponseUnavailable = errors.New("HTTP response is no longer available")

func (v httpResponseView) useWriter(use func(httpcontract.ResponseWriter)) (available bool) {
	v.input.read(func(s Sources) {
		if s.HTTPResponse == nil {
			return
		}
		if request, ok := s.HTTPResponse.(interface {
			UseResponseWriter(func(httpcontract.ResponseWriter)) bool
		}); ok {
			available = request.UseResponseWriter(use)
			return
		}
		writer := s.HTTPResponse.ResponseWriter()
		if writer != nil {
			use(writer)
			available = true
		}
	})
	return
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
