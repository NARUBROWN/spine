package execution

import (
	"encoding/json"
	"mime/multipart"
	"net/textproto"
	"reflect"
	"testing"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
)

type requestOnlyContext struct {
	testExecutionContext
	form *multipart.Form
}

func (*requestOnlyContext) Param(string) string { return "42" }
func (*requestOnlyContext) Query(string) string { return "active" }
func (*requestOnlyContext) Headers() map[string][]string {
	return map[string][]string{"X-Request-ID": {"request"}}
}
func (*requestOnlyContext) Bind(any, ...[]string) error               { return nil }
func (c *requestOnlyContext) MultipartForm() (*multipart.Form, error) { return c.form, nil }

type responseOnlyContext struct {
	testExecutionContext
	writer httpcontract.ResponseWriter
}

func (c *responseOnlyContext) ResponseWriter() httpcontract.ResponseWriter     { return c.writer }
func (c *responseOnlyContext) SetResponseWriter(w httpcontract.ResponseWriter) { c.writer = w }

type viewWriter struct {
	headers   map[string]string
	committed bool
	status    int
	body      string
}

func (w *viewWriter) SetHeader(k, v string)   { w.headers[k] = v }
func (w *viewWriter) AddHeader(k, v string)   { w.headers[k] += v }
func (w *viewWriter) IsCommitted() bool       { return w.committed }
func (w *viewWriter) WriteStatus(s int) error { w.status = s; return nil }
func (w *viewWriter) WriteJSON(s int, v json.RawMessage) error {
	w.status = s
	w.body = string(v)
	return nil
}
func (w *viewWriter) WriteString(s int, v string) error { w.status = s; w.body = v; return nil }
func (w *viewWriter) WriteBytes(s int, v []byte) error  { w.status = s; w.body = string(v); return nil }

func TestHTTPViewsHaveIndependentCapabilities(t *testing.T) {
	requestSource := &requestOnlyContext{form: &multipart.Form{
		Value: map[string][]string{"title": {"original"}},
		File:  map[string][]*multipart.FileHeader{"file": {{Filename: "original.txt", Header: textproto.MIMEHeader{"Content-Type": {"text/plain"}}}}},
	}}
	public := NewContext(requestSource)
	request, ok := public.HTTPRequestContext()
	if !ok || request.Method() != "GET" || request.Path() != "/" || request.Param("id") != "42" || request.Query("state") != "active" {
		t.Fatal("응답 계약 없이 HTTP 요청 뷰를 조회하지 못했습니다")
	}
	if response, ok := public.HTTPResponseContext(); ok || response != nil {
		t.Fatal("요청 전용 컨텍스트에 응답 뷰가 생겼습니다")
	}
	form, err := request.MultipartForm()
	if err != nil {
		t.Fatal(err)
	}
	form.Value["title"][0] = "changed"
	form.File["file"][0].Filename = "changed.txt"
	form.File["file"][0].Header.Set("Content-Type", "changed")
	again, _ := request.MultipartForm()
	if again.Value["title"][0] != "original" || again.File["file"][0].Filename != "original.txt" || again.File["file"][0].Header.Get("Content-Type") != "text/plain" {
		t.Fatal("공개 멀티파트 뷰가 내부 원본을 변경했습니다")
	}

	responseSource := &responseOnlyContext{writer: &viewWriter{headers: map[string]string{}}}
	publicResponse := NewContext(responseSource)
	response, ok := publicResponse.HTTPResponseContext()
	if !ok {
		t.Fatal("요청 계약 없이 HTTP 응답 뷰를 조회하지 못했습니다")
	}
	if request, ok := publicResponse.HTTPRequestContext(); ok || request != nil {
		t.Fatal("응답 전용 컨텍스트에 요청 뷰가 생겼습니다")
	}
	for _, view := range []any{public, request, response} {
		for _, name := range []string{"HTTP", "Bind", "ResponseWriter", "SetResponseWriter", "ReleaseStore", "EventBus"} {
			if _, found := reflect.TypeOf(view).MethodByName(name); found {
				t.Fatalf("공개 뷰 %T에 내부 메서드가 노출됐습니다: %s", view, name)
			}
		}
	}
	responseSource.writer = nil
	if response, ok := publicResponse.HTTPResponseContext(); ok || response != nil {
		t.Fatal("writer 없는 컨텍스트는 응답 뷰를 제공하면 안 됩니다")
	}
}

func TestRetainedHTTPResponseViewFollowsWriterReplacement(t *testing.T) {
	first := &viewWriter{headers: map[string]string{}}
	second := &viewWriter{headers: map[string]string{}, committed: true}
	source := &responseOnlyContext{writer: first}
	response, ok := NewContext(source).HTTPResponseContext()
	if !ok {
		t.Fatal("응답 뷰가 없습니다")
	}
	response.SetHeader("X-Phase", "first")
	source.SetResponseWriter(second)
	response.SetHeader("X-Phase", "second")
	response.AddHeader("X-Phase", "-current")
	if first.headers["X-Phase"] != "first" || second.headers["X-Phase"] != "second-current" || !response.IsCommitted() {
		t.Fatal("보관한 뷰가 교체된 writer를 참조하지 않습니다")
	}
	response.WriteStatus(201)
	response.WriteJSON(202, json.RawMessage(`{"phase":"json"}`))
	if second.status != 202 || second.body != `{"phase":"json"}` {
		t.Fatal("JSON 응답이 현재 writer에 전달되지 않았습니다")
	}
	response.WriteString(203, "string")
	if second.status != 203 || second.body != "string" {
		t.Fatal("문자열 응답이 현재 writer에 전달되지 않았습니다")
	}
	response.WriteBytes(206, []byte("bytes"))
	if first.status != 0 || second.status != 206 || second.body != "bytes" {
		t.Fatal("본문 쓰기가 이전 writer로 전달됐습니다")
	}
}
