package handler

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

type fakeExecutionContext struct {
	writer core.ResponseWriter
	store  map[string]any
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
	jsonBody         json.RawMessage
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
func (w *fakeResponseWriter) WriteJSON(status int, value json.RawMessage) error {
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
	ctx.SetResponseWriter(writer)

	resp := httpx.Response[int]{
		Body: 7,
		Options: httpx.ResponseOptions{
			Status:  201,
			Headers: map[string]string{"X-Test": "1"},
			Cookies: []httpx.Cookie{{Name: "session", Value: "abc", Path: "/", HttpOnly: true}},
		},
	}

	if err := h.Handle(resp, ctx); err != nil {
		t.Fatalf("JSONReturnHandler Handle 실패: %v", err)
	}
	if writer.status != 201 || string(writer.jsonBody) != "7" {
		t.Fatalf("JSON 응답이 잘못되었습니다: status=%d body=%v", writer.status, writer.jsonBody)
	}
	if writer.headers["X-Test"] != "1" {
		t.Fatalf("헤더가 설정되지 않았습니다: %v", writer.headers)
	}
	if len(writer.setCookies) != 1 || writer.setCookies[0] != "session=abc; Path=/; HttpOnly" {
		t.Fatalf("정상 쿠키가 보존되지 않았습니다: %v", writer.setCookies)
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
	ctx.SetResponseWriter(writer)

	resp := httpx.Response[string]{
		Body:    "ok",
		Options: httpx.ResponseOptions{Cookies: []httpx.Cookie{{Name: "theme", Value: "dark"}}},
	}
	if err := h.Handle(resp, ctx); err != nil {
		t.Fatalf("StringReturnHandler Handle 실패: %v", err)
	}
	if writer.stringBody != "ok" {
		t.Fatalf("문자열 응답이 잘못되었습니다: %q", writer.stringBody)
	}
	if len(writer.setCookies) != 1 || writer.setCookies[0] != "theme=dark" {
		t.Fatalf("정상 쿠키가 보존되지 않았습니다: %v", writer.setCookies)
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
	ctx.SetResponseWriter(writer)

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
	ctx.SetResponseWriter(writer)

	bin := httpx.Binary{
		ContentType: "image/png",
		Data:        []byte{1, 2, 3},
		Options: httpx.ResponseOptions{
			Headers: map[string]string{"X-Bin": "yes"},
			Cookies: []httpx.Cookie{{Name: "download", Value: "yes", Secure: true}},
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
	if len(writer.setCookies) != 1 || writer.setCookies[0] != "download=yes; Secure" {
		t.Fatalf("정상 쿠키가 보존되지 않았습니다: %v", writer.setCookies)
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

func TestRedirectReturnHandler_RejectsInjectedCookieWithoutPartialResponse(t *testing.T) {
	h := &RedirectReturnValueHandler{}
	ctx := newFakeExecutionContext()
	writer := newFakeWriter()
	ctx.SetResponseWriter(writer)

	err := h.Handle(httpx.Redirect{
		Location: "/target",
		Options: httpx.ResponseOptions{
			Headers: map[string]string{"X-Partial": "must-not-be-written"},
			Cookies: []httpx.Cookie{
				{Name: "valid-first", Value: "must-not-be-written"},
				{Name: "session", Value: "attacker; Domain=example.com"},
			},
		},
	}, ctx)
	if err == nil {
		t.Fatal("injected cookie attributes must return an explicit error")
	}
	if writer.status != 0 || len(writer.headers) != 0 || len(writer.setCookies) != 0 {
		t.Fatalf("invalid cookie must not produce a partial response: status=%d headers=%v cookies=%v", writer.status, writer.headers, writer.setCookies)
	}
}

func TestResponseHandlers_RejectInvalidCookieWithoutPartialResponse(t *testing.T) {
	invalidOptions := httpx.ResponseOptions{
		Headers: map[string]string{"X-Partial": "must-not-be-written"},
		Cookies: []httpx.Cookie{
			{Name: "valid-first", Value: "must-not-be-written"},
			{Name: "session", Value: "safe\r\nX-Injected: true"},
		},
	}

	tests := []struct {
		name   string
		handle func(core.ExecutionContext) error
	}{
		{
			name: "json",
			handle: func(ctx core.ExecutionContext) error {
				return (&JSONReturnHandler{}).Handle(httpx.Response[int]{Body: 1, Options: invalidOptions}, ctx)
			},
		},
		{
			name: "string",
			handle: func(ctx core.ExecutionContext) error {
				return (&StringReturnHandler{}).Handle(httpx.Response[string]{Body: "ok", Options: invalidOptions}, ctx)
			},
		},
		{
			name: "binary",
			handle: func(ctx core.ExecutionContext) error {
				return (&BinaryReturnHandler{}).Handle(httpx.Binary{Data: []byte("ok"), ContentType: "text/plain", Options: invalidOptions}, ctx)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newFakeExecutionContext()
			writer := newFakeWriter()
			ctx.SetResponseWriter(writer)

			err := tt.handle(ctx)
			if err == nil {
				t.Fatal("invalid cookie must return an explicit error")
			}
			if writer.status != 0 || len(writer.headers) != 0 || len(writer.setCookies) != 0 || writer.writeJSONCalls != 0 || writer.writeStringCalls != 0 || writer.writeBytesCalls != 0 {
				t.Fatalf("invalid cookie must not produce a partial response: status=%d headers=%v cookies=%v writes=%d/%d/%d", writer.status, writer.headers, writer.setCookies, writer.writeJSONCalls, writer.writeStringCalls, writer.writeBytesCalls)
			}
		})
	}
}

func TestSerializeCookieValidated_RejectsInvalidCookieContractValues(t *testing.T) {
	tests := []struct {
		name   string
		cookie httpx.Cookie
	}{
		{name: "empty name", cookie: httpx.Cookie{Value: "value"}},
		{name: "unknown same site", cookie: httpx.Cookie{Name: "session", Value: "value", SameSite: httpx.SameSite("Flexible")}},
		{name: "unknown priority", cookie: httpx.Cookie{Name: "session", Value: "value", Priority: "Urgent"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if cookie, err := serializeCookieValidated(tt.cookie); err == nil {
				t.Fatalf("invalid cookie contract value must fail, got %q", cookie)
			}
		})
	}
}

func TestSerializeCookieValidated_AcceptsDocumentedCookieContractValues(t *testing.T) {
	for _, sameSite := range []httpx.SameSite{"", httpx.SameSiteLax, httpx.SameSiteStrict, httpx.SameSiteNone} {
		for _, priority := range []string{"", "Low", "Medium", "High"} {
			cookie, err := serializeCookieValidated(httpx.Cookie{
				Name:     "session",
				Value:    "value",
				SameSite: sameSite,
				Priority: priority,
			})
			if err != nil {
				t.Fatalf("documented values must remain valid (SameSite=%q Priority=%q): %v", sameSite, priority, err)
			}
			if cookie == "" {
				t.Fatal("valid cookie must serialize to a non-empty value")
			}
		}
	}
}

func TestBinaryReturnHandler_InvalidType(t *testing.T) {
	h := &BinaryReturnHandler{}
	ctx := newFakeExecutionContext()
	writer := newFakeWriter()
	ctx.SetResponseWriter(writer)

	if err := h.Handle("not-binary", ctx); err == nil {
		t.Fatal("잘못된 타입은 에러여야 합니다")
	}
}

func TestRedirectReturnValueHandler_Handle(t *testing.T) {
	h := &RedirectReturnValueHandler{}
	ctx := newFakeExecutionContext()
	writer := newFakeWriter()
	ctx.SetResponseWriter(writer)

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
	if writer.setCookies[0] != "session=abc; Path=/; Expires=Tue, 14 Nov 2023 22:13:20 GMT; HttpOnly; Secure; SameSite=Lax" {
		t.Fatalf("정상 리다이렉트 쿠키가 보존되지 않았습니다: %v", writer.setCookies)
	}

	ptrRedirect := &httpx.Redirect{Location: "/home"}
	if err := h.Handle(ptrRedirect, ctx); err != nil {
		t.Fatalf("포인터 redirect 처리 실패: %v", err)
	}
	if writer.headers["Location"] != "/home" {
		t.Fatalf("포인터 redirect location이 잘못되었습니다: %v", writer.headers)
	}
}

func TestSerializeCookieValidated_ClearCookieUsesPortableExpiration(t *testing.T) {
	cookie, err := serializeCookieValidated(httpx.ClearAccessTokenCookie())
	if err != nil {
		t.Fatalf("삭제 쿠키 직렬화 실패: %v", err)
	}
	want := "accessToken=; Path=/; Max-Age=0; Expires=Thu, 01 Jan 1970 00:00:01 GMT"
	if cookie != want {
		t.Fatalf("삭제 쿠키가 즉시 만료 형식이어야 합니다: got %q want %q", cookie, want)
	}
}

type customErr struct{ msg string }

func (e customErr) Error() string { return e.msg }

func TestErrorReturnHandler_HTTPError(t *testing.T) {
	h := &ErrorReturnHandler{}
	ctx := newFakeExecutionContext()
	writer := newFakeWriter()
	ctx.SetResponseWriter(writer)

	err := httperr.BadRequest("bad")
	if err := h.Handle(err, ctx); err != nil {
		t.Fatalf("ErrorReturnHandler 실패: %v", err)
	}
	if writer.status != 400 {
		t.Fatalf("HTTPError 상태 코드가 잘못되었습니다: %d", writer.status)
	}
	var body map[string]string
	if err := json.Unmarshal(writer.jsonBody, &body); err != nil {
		t.Fatal(err)
	}
	if body["message"] != "bad" {
		t.Fatalf("메시지가 잘못되었습니다: %v", body)
	}
}

func TestErrorReturnHandler_GenericError(t *testing.T) {
	h := &ErrorReturnHandler{}
	ctx := newFakeExecutionContext()
	writer := newFakeWriter()
	ctx.SetResponseWriter(writer)

	if err := h.Handle(customErr{"boom"}, ctx); err != nil {
		t.Fatalf("ErrorReturnHandler 실패: %v", err)
	}
	if writer.status != 500 {
		t.Fatalf("기본 상태 코드는 500이어야 합니다: %d", writer.status)
	}
	var body map[string]string
	if err := json.Unmarshal(writer.jsonBody, &body); err != nil {
		t.Fatal(err)
	}
	if body["message"] != "Internal server error" {
		t.Fatalf("메시지가 잘못되었습니다: %v", body)
	}
	if body["message"] == "boom" {
		t.Fatal("일반 오류 원문이 응답에 노출되었습니다")
	}
}

func (c *fakeExecutionContext) SetRoute(params map[string]string, pathKeys []string) {}

func (c *fakeExecutionContext) ResponseWriter() core.ResponseWriter          { return c.writer }
func (c *fakeExecutionContext) SetResponseWriter(writer core.ResponseWriter) { c.writer = writer }
