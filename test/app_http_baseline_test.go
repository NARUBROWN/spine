package test

import httpinput "github.com/NARUBROWN/spine/pkg/input"

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
	spinemultipart "github.com/NARUBROWN/spine/pkg/multipart"
)

type httpBaselineDTO struct {
	Name  string `json:"name" xml:"name" validate:"required"`
	Count int    `json:"count" xml:"count"`
}

type httpBaselineForm struct {
	Name  string `json:"name" form:"name"`
	Count int    `json:"count" form:"count"`
}

type httpBaselineFile struct {
	FieldName   string `json:"field_name"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Content     string `json:"content"`
}

type httpBaselineUpload struct {
	Form  httpBaselineForm   `json:"form"`
	Files []httpBaselineFile `json:"files"`
}

type httpBaselinePath struct {
	ID int64 `path:"id"`
}
type httpBaselineController struct{}

func (*httpBaselineController) Bind(dto httpinput.Body[httpBaselineDTO]) httpx.Response[httpBaselineDTO] {
	return httpx.Response[httpBaselineDTO]{Body: *dto}
}

func (*httpBaselineController) Form(dto httpinput.Body[httpBaselineForm]) httpx.Response[httpBaselineForm] {
	return httpx.Response[httpBaselineForm]{Body: *dto}
}

func (*httpBaselineController) Path(p httpinput.Path[httpBaselinePath]) httpx.Response[map[string]int] {
	return httpx.Response[map[string]int]{Body: map[string]int{
		"id": int(p.ID),
	}}
}

func (*httpBaselineController) Upload(form httpinput.Body[httpBaselineForm], files spinemultipart.UploadedFiles) (httpx.Response[httpBaselineUpload], error) {
	body := httpBaselineUpload{Form: *form}
	for _, file := range files.Files {
		reader, err := file.Open()
		if err != nil {
			return httpx.Response[httpBaselineUpload]{}, err
		}
		content, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return httpx.Response[httpBaselineUpload]{}, readErr
		}
		if closeErr != nil {
			return httpx.Response[httpBaselineUpload]{}, closeErr
		}
		body.Files = append(body.Files, httpBaselineFile{
			FieldName: file.FieldName, Filename: file.Filename, ContentType: file.ContentType,
			Size: file.Size, Content: string(content),
		})
	}
	return httpx.Response[httpBaselineUpload]{Body: body}, nil
}

// 기준 요청은 공개 Prepare API로 구성합니다. 리스너나 관리 실행 고루틴을 시작하지 않습니다.
func newHTTPBaselineHandler(t *testing.T, app spine.App, options boot.HTTPOptions) http.Handler {
	t.Helper()
	prepared, err := app.Prepare(context.Background(), boot.Options{
		ShutdownTimeout: 3 * time.Second, HTTP: &options,
	})
	if err != nil {
		t.Fatalf("기준 앱 준비 실패: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := prepared.Close(ctx); err != nil {
			t.Errorf("기준 앱 종료 실패: %v", err)
		}
	})
	return mustHTTPHandler(t, prepared)
}

func setupHTTPBaselineApp(options ...platform.EchoOption) spine.App {
	app := platform.New(options...)
	app.Constructor(func() *httpBaselineController { return &httpBaselineController{} })
	for _, method := range []string{http.MethodGet, http.MethodDelete, http.MethodPost} {
		app.Route(method, "/body", (*httpBaselineController).Bind)
	}
	app.Route(http.MethodPost, "/form", (*httpBaselineController).Form)
	app.Route(http.MethodPost, "/upload", (*httpBaselineController).Upload)
	app.Route(http.MethodGet, "/items/:id", (*httpBaselineController).Path)
	return app
}

func TestAppIntegration_HTTPBindingBaseline(t *testing.T) {
	handler := newHTTPBaselineHandler(t, setupHTTPBaselineApp(), boot.HTTPOptions{})
	cases := []struct {
		name, method, target, contentType, body string
		status                                  int
		want                                    httpBaselineDTO
		message                                 string
	}{
		{name: "json", method: "POST", target: "/body", contentType: "application/json; charset=utf-8", body: `{"name":"json","count":2}`, status: 200, want: httpBaselineDTO{Name: "json", Count: 2}},
		{name: "xml", method: "POST", target: "/body", contentType: "application/xml", body: `<item><name>xml</name><count>3</count></item>`, status: 200, want: httpBaselineDTO{Name: "xml", Count: 3}},
		{name: "text_xml", method: "POST", target: "/body", contentType: "text/xml", body: `<item><name>xml</name><count>4</count></item>`, status: 200, want: httpBaselineDTO{Name: "xml", Count: 4}},
		{name: "body_dto_does_not_bind_get_query", method: "GET", target: "/body?name=query&count=5", status: 200},
		{name: "body_dto_does_not_bind_delete_query", method: "DELETE", target: "/body?name=query&count=6", status: 200},
		{name: "get_body_dto_uses_body_only", method: "GET", target: "/body?name=query&count=7", contentType: "application/json", body: `{"name":"body","count":8}`, status: 200, want: httpBaselineDTO{Name: "body", Count: 8}},
		{name: "post_does_not_bind_query", method: "POST", target: "/body?name=query&count=9", status: 200},
		{name: "post_body", method: "POST", target: "/body?name=query&count=9", contentType: "application/json", body: `{"name":"body","count":10}`, status: 200, want: httpBaselineDTO{Name: "body", Count: 10}},
		{name: "required_tag_is_not_validation", method: "POST", target: "/body", contentType: "application/json", body: `{}`, status: 200},
		{name: "unknown_json_field_is_ignored", method: "POST", target: "/body", contentType: "application/json", body: `{"name":"known","extra":true}`, status: 200, want: httpBaselineDTO{Name: "known"}},
		{name: "malformed_json", method: "POST", target: "/body", contentType: "application/json", body: `{`, status: 400, message: "Bad Request"},
		{name: "invalid_field_type", method: "POST", target: "/body", contentType: "application/json", body: `{"count":"invalid"}`, status: 400, message: "Bad Request"},
		{name: "malformed_xml", method: "POST", target: "/body", contentType: "application/xml", body: `<item>`, status: 400, message: "Bad Request"},
		{name: "unsupported_type", method: "POST", target: "/body", contentType: "text/plain", body: "payload", status: 415, message: "Unsupported Media Type"},
		{name: "missing_content_type", method: "POST", target: "/body", body: `{}`, status: 415, message: "Unsupported Media Type"},
		{name: "empty_body_skips_content_type", method: "POST", target: "/body", contentType: "text/plain", status: 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body))
			if tc.contentType != "" {
				request.Header.Set("Content-Type", tc.contentType)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != tc.status {
				t.Fatalf("응답 상태 = %d, 기대값 = %d, 본문 = %s", recorder.Code, tc.status, recorder.Body.String())
			}
			if tc.message != "" {
				var got struct{ Message string }
				if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil || got.Message != tc.message {
					t.Fatalf("입력 오류 응답 = %s, 기대 메시지 = %q, 파싱 오류 = %v", recorder.Body.String(), tc.message, err)
				}
				return
			}
			var got httpBaselineDTO
			if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil || got != tc.want {
				t.Fatalf("바인딩 결과 = %+v, 기대값 = %+v, 파싱 오류 = %v", got, tc.want, err)
			}
		})
	}
}

func TestAppIntegration_FormBindingBaseline(t *testing.T) {
	handler := newHTTPBaselineHandler(t, setupHTTPBaselineApp(), boot.HTTPOptions{})
	request := httptest.NewRequest(http.MethodPost, "/form?name=query&count=99", strings.NewReader("name=form&count=7"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var got httpBaselineForm
	want := httpBaselineForm{Name: "form", Count: 7}
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); recorder.Code != 200 || err != nil || got != want {
		t.Fatalf("폼 본문이 쿼리보다 우선해야 합니다: 상태=%d 결과=%+v 기대값=%+v 오류=%v", recorder.Code, got, want, err)
	}
}

func TestAppIntegration_MultipartBindingBaseline(t *testing.T) {
	handler := newHTTPBaselineHandler(t, setupHTTPBaselineApp(), boot.HTTPOptions{})
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"name": "multipart", "count": "2"} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	wantFiles := []httpBaselineFile{
		{FieldName: "files", Filename: "first.txt", ContentType: "application/octet-stream", Size: 5, Content: "first"},
		{FieldName: "files", Filename: "second.txt", ContentType: "application/octet-stream", Size: 6, Content: "second"},
	}
	for _, file := range wantFiles {
		part, err := writer.CreateFormFile(file.FieldName, file.Filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, file.Content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	t.Cleanup(func() {
		if request.MultipartForm != nil {
			if err := request.MultipartForm.RemoveAll(); err != nil {
				t.Errorf("멀티파트 임시 파일 정리 실패: %v", err)
			}
		}
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var got httpBaselineUpload
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); recorder.Code != 200 || err != nil {
		t.Fatalf("멀티파트 응답: 상태=%d 본문=%s 오류=%v", recorder.Code, recorder.Body.String(), err)
	}
	if got.Form != (httpBaselineForm{Name: "multipart", Count: 2}) || len(got.Files) != len(wantFiles) {
		t.Fatalf("폼과 파일을 함께 해석해야 합니다: %+v", got)
	}
	for index, want := range wantFiles {
		if got.Files[index] != want {
			t.Errorf("파일 %d: 결과=%+v 기대값=%+v", index, got.Files[index], want)
		}
	}
}

func TestAppIntegration_PathBindingBaseline(t *testing.T) {
	handler := newHTTPBaselineHandler(t, setupHTTPBaselineApp(), boot.HTTPOptions{})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/items/43", nil))
	var got map[string]int
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); recorder.Code != 200 || err != nil {
		t.Fatalf("경로 응답: 상태=%d 본문=%s 오류=%v", recorder.Code, recorder.Body.String(), err)
	}
	if got["id"] != 43 {
		t.Fatalf("경로 값은 기존 Spine 경로 인자로 읽어야 합니다: %+v", got)
	}
}
