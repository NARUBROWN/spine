package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platform "github.com/NARUBROWN/spine-echo-platform"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
	files "github.com/NARUBROWN/spine/pkg/multipart"
	"github.com/NARUBROWN/spine/pkg/route"
	"github.com/labstack/echo/v5"
)

type inputV3Form struct {
	Title string `form:"title"`
}

type inputV3Controller struct {
	t            *testing.T
	fail         string
	calls        atomic.Int32
	path         string
	order        []string
	executionErr error
}

func (c *inputV3Controller) FormFirst(form *inputV3Form, upload files.UploadedFiles) httpx.Response[string] {
	return c.handle(form, upload)
}

func (c *inputV3Controller) FilesFirst(upload files.UploadedFiles, form *inputV3Form) httpx.Response[string] {
	return c.handle(form, upload)
}

func (c *inputV3Controller) handle(form *inputV3Form, upload files.UploadedFiles) httpx.Response[string] {
	c.calls.Add(1)
	c.order = append(c.order, "controller")
	if form.Title != "body" || len(upload.Files) != 1 {
		c.t.Errorf("폼과 파일은 같은 본문에서 읽어야 합니다: %+v %+v", form, upload)
		return httpx.Response[string]{}
	}
	f, err := upload.Files[0].Open()
	if err != nil {
		c.t.Error(err)
		return httpx.Response[string]{}
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil || string(data) != strings.Repeat("payload", 100) {
		c.t.Errorf("파일 내용: %q %v", data, err)
	}
	if disk, ok := f.(*os.File); ok {
		c.path = disk.Name()
	} else {
		c.t.Error("작은 메모리 예산을 넘은 파일은 디스크에 있어야 합니다")
	}
	if c.fail == "panic" {
		panic("controller panic")
	}
	return httpx.Response[string]{Body: form.Title}
}

type inputV3Interceptor struct {
	controller *inputV3Controller
	reject     bool
	failAfter  bool
}

func (i *inputV3Interceptor) PreHandle(core.ExecutionContext, core.HandlerMeta) error {
	i.controller.order = append(i.controller.order, "pre")
	if i.reject {
		return httperr.Unauthorized("authentication required")
	}
	return nil
}

func (i *inputV3Interceptor) BeforeResponse(_ core.ExecutionContext, _ core.HandlerMeta, _ error) error {
	i.controller.order = append(i.controller.order, "before")
	if i.controller.fail == "before" {
		return errors.New("before response failed")
	}
	return nil
}

func (i *inputV3Interceptor) AfterCompletion(ctx core.ExecutionContext, _ core.HandlerMeta, executionErr error) {
	c := i.controller
	c.executionErr = executionErr
	c.order = append(c.order, "after")
	if c.path == "" {
		return
	}
	if _, err := os.Stat(c.path); err != nil {
		c.t.Errorf("AfterCompletion까지 파일이 있어야 합니다: %v", err)
	}
	form, err := ctx.(core.HttpRequestContext).MultipartForm()
	if err != nil || form.Value["title"][0] != "body" {
		c.t.Errorf("완료 훅에서 같은 multipart 결과를 읽어야 합니다: %v", err)
	}
	if i.failAfter {
		panic("completion panic")
	}
}

func TestHTTPV3MultipartLifetimeThroughActualPipeline(t *testing.T) {
	for _, first := range []bool{true, false} {
		for _, failure := range []string{"", "panic", "before", "after"} {
			t.Run(map[bool]string{true: "form-first", false: "files-first"}[first]+"/"+failure, func(t *testing.T) {
				t.Setenv("TMPDIR", t.TempDir())
				c := &inputV3Controller{t: t, fail: failure}
				complete := make(chan struct{})
				app := platform.New(platform.WithEcho(func(e *echo.Echo) {
					e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
						return func(ctx *echo.Context) error {
							defer close(complete)
							err := next(ctx)
							if c.path != "" {
								if _, statErr := os.Stat(c.path); !errors.Is(statErr, os.ErrNotExist) {
									t.Errorf("Execute 반환 전 파일을 제거해야 합니다: %v", statErr)
								}
							}
							return err
						}
					})
				}))
				app.Constructor(func() *inputV3Controller { return c })
				interceptor := &inputV3Interceptor{controller: c, failAfter: failure == "after"}
				if first {
					app.Route("POST", "/upload", (*inputV3Controller).FormFirst, route.WithInterceptors(interceptor))
				} else {
					app.Route("POST", "/upload", (*inputV3Controller).FilesFirst, route.WithInterceptors(interceptor))
				}
				p, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{MultipartMemoryBytes: 1}})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = p.Close(context.Background()) })
				srv := httptest.NewServer(mustHTTPHandler(t, p))
				t.Cleanup(srv.Close)
				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				_ = writer.WriteField("title", "body")
				part, err := writer.CreateFormFile("files", "file.txt")
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.WriteString(part, strings.Repeat("payload", 100))
				_ = writer.Close()
				response, err := srv.Client().Post(srv.URL+"/upload?title=query", writer.FormDataContentType(), &body)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
				select {
				case <-complete:
				case <-time.After(3 * time.Second):
					t.Fatal("요청 완료 시간 초과")
				}
				wantStatus := 200
				if failure == "panic" || failure == "before" {
					wantStatus = 500
				}
				if response.StatusCode != wantStatus || c.calls.Load() != 1 || strings.Join(c.order, ",") != "pre,controller,before,after" {
					t.Fatalf("상태/순서: %d %v", response.StatusCode, c.order)
				}
				remaining, err := filepath.Glob(filepath.Join(os.TempDir(), "multipart-*"))
				if err != nil || len(remaining) != 0 {
					t.Fatalf("남은 임시 파일: %v %v", remaining, err)
				}
			})
		}
	}
}

type countInputBody struct{ reads atomic.Int32 }

func (b *countInputBody) Read([]byte) (int, error) {
	b.reads.Add(1)
	return 0, errors.New("body must not be read")
}
func (*countInputBody) Close() error { return nil }

func TestHTTPV3AuthenticationRejectsBeforeCommonBodyRead(t *testing.T) {
	for _, global := range []bool{true, false} {
		t.Run(map[bool]string{true: "global", false: "route"}[global], func(t *testing.T) {
			c := &inputV3Controller{t: t}
			app := platform.New()
			app.Constructor(func() *inputV3Controller { return c })
			i := &inputV3Interceptor{controller: c, reject: true}
			if global {
				app.Interceptor(i)
				app.Route("POST", "/upload", (*inputV3Controller).FormFirst)
			} else {
				app.Route("POST", "/upload", (*inputV3Controller).FormFirst, route.WithInterceptors(i))
			}
			p, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = p.Close(context.Background()) })
			body := &countInputBody{}
			request := httptest.NewRequest("POST", "/upload", nil)
			request.Body = body
			request.Header.Set("Content-Type", "multipart/form-data; boundary=broken")
			response := httptest.NewRecorder()
			mustHTTPHandler(t, p).ServeHTTP(response, request)
			if response.Code != 401 || body.reads.Load() != 0 || c.calls.Load() != 0 {
				t.Fatalf("인증/읽기/컨트롤러: %d/%d/%d", response.Code, body.reads.Load(), c.calls.Load())
			}
		})
	}
}

func (*inputV3Interceptor) PostHandle(core.ExecutionContext, core.HandlerMeta) {}

func (c *inputV3Controller) FileOnly(files.UploadedFiles) httpx.Response[string] {
	c.calls.Add(1)
	return httpx.Response[string]{}
}
func (c *inputV3Controller) JSON(*httpBaselineDTO) httpx.Response[string] {
	c.calls.Add(1)
	return httpx.Response[string]{}
}

type inputV3BrokenReader struct{ err error }

func (r inputV3BrokenReader) Read([]byte) (int, error) { return 0, r.err }

func TestHTTPV3InputFailuresPreserveSourceAndCause(t *testing.T) {
	failure := errors.New("original body read failure")
	for _, tc := range []struct {
		name, content, data string
		max                 int64
		broken              error
		files               bool
		status              int
	}{
		{name: "file-only-invalid-multipart", content: "multipart/form-data; boundary=missing", data: "malformed", files: true, status: 400},
		{name: "file-only-body-limit", content: "multipart/form-data; boundary=missing", data: strings.Repeat("x", 64), max: 1, files: true, status: 413},
		{name: "body-media-type", content: "text/plain", data: "body", status: 415},
		{name: "body-read-error", content: "application/json", broken: failure, status: 500},
		{name: "body-cancel", content: "application/json", data: `{}`, broken: context.Canceled, status: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &inputV3Controller{t: t}
			app := platform.New()
			app.Constructor(func() *inputV3Controller { return c })
			interceptor := &inputV3Interceptor{controller: c}
			if tc.files {
				app.Route("POST", "/input", (*inputV3Controller).FileOnly, route.WithInterceptors(interceptor))
			} else {
				app.Route("POST", "/input", (*inputV3Controller).JSON, route.WithInterceptors(interceptor))
			}
			p, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{MaxBodyBytes: tc.max}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = p.Close(context.Background()) })
			request := httptest.NewRequest("POST", "/input", strings.NewReader(tc.data))
			request.Header.Set("Content-Type", tc.content)
			if tc.broken != nil {
				request.Body = io.NopCloser(inputV3BrokenReader{tc.broken})
			}
			if tc.broken == context.Canceled {
				ctx, cancel := context.WithCancel(request.Context())
				cancel()
				request = request.WithContext(ctx)
			}
			response := httptest.NewRecorder()
			mustHTTPHandler(t, p).ServeHTTP(response, request)
			if response.Code != tc.status || c.calls.Load() != 0 {
				t.Fatalf("오류 입력 실행: %d %s", response.Code, response.Body.String())
			}
			var source *httperr.InputError
			if !errors.As(c.executionErr, &source) || source.Source != "body" {
				t.Fatalf("입력 출처를 보존해야 합니다: %v", c.executionErr)
			}
			if tc.broken != nil && !errors.Is(c.executionErr, tc.broken) {
				t.Fatalf("원래 읽기/취소 원인: %v", c.executionErr)
			}
			if tc.status < 500 {
				var body struct {
					In string `json:"in"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.In != "body" {
					t.Fatalf("공개 입력 출처: %s %v", response.Body.String(), err)
				}
			}
		})
	}
}
