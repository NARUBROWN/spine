package handler

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

type fakeExecutionContext struct {
	store map[string]any
}

func newFakeExecutionContext() *fakeExecutionContext {
	return &fakeExecutionContext{store: map[string]any{}}
}

func (c *fakeExecutionContext) Context() context.Context     { return context.Background() }
func (c *fakeExecutionContext) EventBus() core.EventBus      { return nil }
func (c *fakeExecutionContext) Method() string               { return "" }
func (c *fakeExecutionContext) Path() string                 { return "" }
func (c *fakeExecutionContext) Params() map[string]string    { return nil }
func (c *fakeExecutionContext) Header(name string) string    { return "" }
func (c *fakeExecutionContext) PathKeys() []string           { return nil }
func (c *fakeExecutionContext) Queries() map[string][]string { return nil }
func (c *fakeExecutionContext) Set(key string, value any)    { c.store[key] = value }
func (c *fakeExecutionContext) Get(key string) (any, bool)   { v, ok := c.store[key]; return v, ok }

type fakeResponseWriter struct {
	headers          map[string]string
	setCookies       []string
	status           int
	jsonBody         any
	stringBody       string
	bytesBody        []byte
	writeJSONCalls   int
	writeStringCalls int
	writeBytesCalls  int
}

func newFakeWriter() *fakeResponseWriter {
	return &fakeResponseWriter{headers: map[string]string{}}
}

func (w *fakeResponseWriter) SetHeader(key, value string) { w.headers[key] = value }
func (w *fakeResponseWriter) AddHeader(key, value string) {
	if key == "Set-Cookie" {
		w.setCookies = append(w.setCookies, value)
		return
	}
	w.headers[key] = value
}
func (w *fakeResponseWriter) IsCommitted() bool { return false }
func (w *fakeResponseWriter) WriteStatus(status int) error {
	w.status = status
	return nil
}
func (w *fakeResponseWriter) WriteJSON(status int, value any) error {
	w.status = status
	w.jsonBody = value
	w.writeJSONCalls++
	return nil
}
func (w *fakeResponseWriter) WriteString(status int, value string) error {
	w.status = status
	w.stringBody = value
	w.writeStringCalls++
	return nil
}
func (w *fakeResponseWriter) WriteBytes(status int, value []byte) error {
	w.status = status
	w.bytesBody = value
	w.writeBytesCalls++
	return nil
}

func TestJSONReturnHandler_SupportsBoundary(t *testing.T) {
	h := &JSONReturnHandler{}
	if !h.Supports(reflect.TypeOf(httpx.Response[int]{})) {
		t.Fatal("int Response는 JSONReturnHandler가 지원해야 합니다")
	}
	if h.Supports(reflect.TypeOf(httpx.Response[string]{})) {
		t.Fatal("string Response는 JSONReturnHandler가 지원하면 안 됩니다")
	}
}

func TestJSONReturnHandler_Handle(t *testing.T) {
	h := &JSONReturnHandler{}
	ctx := newFakeExecutionContext()
	writer := newFakeWriter()
	ctx.Set("spine.response_writer", writer)

	resp := httpx.Response[int]{
		Body: 7,
		Options: httpx.ResponseOptions{
			Status:  201,
			Headers: map[string]string{"X-Test": "1"},
		},
	}

	if err := h.Handle(resp, ctx); err != nil {
		t.Fatalf("JSONReturnHandler Handle 실패: %v", err)
	}
	if writer.status != 201 || writer.jsonBody != 7 {
		t.Fatalf("JSON 응답이 잘못되었습니다: status=%d body=%v", writer.status, writer.jsonBody)
	}
	if writer.headers["X-Test"] != "1" {
		t.Fatalf("헤더가 설정되지 않았습니다: %v", writer.headers)
	}
}

func TestStringReturnHandler_SupportsAndHandle(t *testing.T) {
	h := &StringReturnHandler{}
	if !h.Supports(reflect.TypeOf(httpx.Response[string]{})) {
		t.Fatal("string Response는 StringReturnHandler가 지원해야 합니다")
	}
	if h.Supports(reflect.TypeOf(httpx.Response[int]{})) {
		t.Fatal("int Response는 StringReturnHandler가 지원하면 안 됩니다")
	}

	ctx := newFakeExecutionContext()
	writer := newFakeWriter()
	ctx.Set("spine.response_writer", writer)

	resp := httpx.Response[string]{Body: "ok"}
	if err := h.Handle(resp, ctx); err != nil {
		t.Fatalf("StringReturnHandler Handle 실패: %v", err)
	}
	if writer.stringBody != "ok" {
		t.Fatalf("문자열 응답이 잘못되었습니다: %q", writer.stringBody)
	}

	ptrResp := &httpx.Response[string]{Body: "ptr"}
	if err := h.Handle(ptrResp, ctx); err != nil {
		t.Fatalf("포인터 응답 처리 실패: %v", err)
	}
	if writer.stringBody != "ptr" {
		t.Fatalf("포인터 문자열 응답이 잘못되었습니다: %q", writer.stringBody)
	}
}

func TestStringReturnHandler_InvalidValues(t *testing.T) {
	h := &StringReturnHandler{}
	ctx := newFakeExecutionContext()
	writer := newFakeWriter()
	ctx.Set("spine.response_writer", writer)

	if err := h.Handle((*httpx.Response[string])(nil), ctx); err == nil {
		t.Fatal("nil 포인터는 에러여야 합니다")
	}
	if err := h.Handle(httpx.Response[int]{Body: 1}, ctx); err == nil {
		t.Fatal("잘못된 타입은 에러여야 합니다")
	}
}

func TestBinaryReturnHandler_Handle(t *testing.T) {
	h := &BinaryReturnHandler{}
	ctx := newFakeExecutionContext()
	writer := newFakeWriter()
	ctx.Set("spine.response_writer", writer)

	bin := httpx.Binary{
		ContentType: "image/png",
		Data:        []byte{1, 2, 3},
		Options: httpx.ResponseOptions{
			Headers: map[string]string{"X-Bin": "yes"},
		},
	}

	if !h.Supports(reflect.TypeOf(bin)) {
		t.Fatal("BinaryReturnHandler가 httpx.Binary를 지원해야 합니다")
	}

	if err := h.Handle(bin, ctx); err != nil {
		t.Fatalf("BinaryReturnHandler Handle 실패: %v", err)
	}
	if writer.status != 200 || string(writer.bytesBody) != string([]byte{1, 2, 3}) {
		t.Fatalf("바이너리 응답이 잘못되었습니다: status=%d body=%v", writer.status, writer.bytesBody)
	}
	if writer.headers["Content-Type"] != "image/png" || writer.headers["X-Bin"] != "yes" {
		t.Fatalf("헤더가 설정되지 않았습니다: %v", writer.headers)
	}

	ptrBin := &httpx.Binary{
		ContentType: "application/octet-stream",
		Data:        []byte{4, 5, 6},
	}
	if err := h.Handle(ptrBin, ctx); err != nil {
		t.Fatalf("포인터 binary 처리 실패: %v", err)
	}
	if string(writer.bytesBody) != string([]byte{4, 5, 6}) {
		t.Fatalf("포인터 binary 응답이 잘못되었습니다: %v", writer.bytesBody)
	}
}

func TestBinaryReturnHandler_InvalidType(t *testing.T) {
	h := &BinaryReturnHandler{}
	ctx := newFakeExecutionContext()
	writer := newFakeWriter()
	ctx.Set("spine.response_writer", writer)

	if err := h.Handle("not-binary", ctx); err == nil {
		t.Fatal("잘못된 타입은 에러여야 합니다")
	}
}

func TestRedirectReturnValueHandler_Handle(t *testing.T) {
	h := &RedirectReturnValueHandler{}
	ctx := newFakeExecutionContext()
	writer := newFakeWriter()
	ctx.Set("spine.response_writer", writer)

	expires := time.Unix(1700000000, 0)
	redirect := httpx.Redirect{
		Location: "/login",
		Options: httpx.ResponseOptions{
			Status: 307,
			Headers: map[string]string{
				"X-Redirect": "1",
			},
			Cookies: []httpx.Cookie{
				{
					Name:     "session",
					Value:    "abc",
					Path:     "/",
					HttpOnly: true,
					Secure:   true,
					SameSite: httpx.SameSiteLax,
					Expires:  &expires,
				},
			},
		},
	}

	if !h.Supports(reflect.TypeOf(redirect)) {
		t.Fatal("RedirectReturnValueHandler가 Redirect를 지원해야 합니다")
	}

	if err := h.Handle(redirect, ctx); err != nil {
		t.Fatalf("RedirectReturnValueHandler Handle 실패: %v", err)
	}
	if writer.status != 307 {
		t.Fatalf("상태 코드가 잘못되었습니다: %d", writer.status)
	}
	if writer.headers["Location"] != "/login" || writer.headers["X-Redirect"] != "1" {
		t.Fatalf("리다이렉트 헤더가 잘못되었습니다: %v", writer.headers)
	}
	if len(writer.setCookies) != 1 {
		t.Fatalf("쿠키가 기록되어야 합니다: %v", writer.setCookies)
	}

	ptrRedirect := &httpx.Redirect{Location: "/home"}
	if err := h.Handle(ptrRedirect, ctx); err != nil {
		t.Fatalf("포인터 redirect 처리 실패: %v", err)
	}
	if writer.headers["Location"] != "/home" {
		t.Fatalf("포인터 redirect location이 잘못되었습니다: %v", writer.headers)
	}
}

type customErr struct{ msg string }

func (e customErr) Error() string { return e.msg }

func TestErrorReturnHandler_HTTPError(t *testing.T) {
	h := &ErrorReturnHandler{}
	ctx := newFakeExecutionContext()
	writer := newFakeWriter()
	ctx.Set("spine.response_writer", writer)

	err := httperr.BadRequest("bad")
	if err := h.Handle(err, ctx); err != nil {
		t.Fatalf("ErrorReturnHandler 실패: %v", err)
	}
	if writer.status != 400 {
		t.Fatalf("HTTPError 상태 코드가 잘못되었습니다: %d", writer.status)
	}
	body := writer.jsonBody.(map[string]any)
	if body["message"] != "bad" {
		t.Fatalf("메시지가 잘못되었습니다: %v", body)
	}
}

func TestErrorReturnHandler_GenericError(t *testing.T) {
	h := &ErrorReturnHandler{}
	ctx := newFakeExecutionContext()
	writer := newFakeWriter()
	ctx.Set("spine.response_writer", writer)

	if err := h.Handle(customErr{"boom"}, ctx); err != nil {
		t.Fatalf("ErrorReturnHandler 실패: %v", err)
	}
	if writer.status != 500 {
		t.Fatalf("기본 상태 코드는 500이어야 합니다: %d", writer.status)
	}
	body := writer.jsonBody.(map[string]any)
	if body["message"] != "Internal server error" {
		t.Fatalf("메시지가 잘못되었습니다: %v", body)
	}
	if body["message"] == "boom" {
		t.Fatal("일반 오류 원문이 응답에 노출되었습니다")
	}
}
