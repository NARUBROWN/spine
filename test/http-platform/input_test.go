package httpplatform_test

import httpinput "github.com/NARUBROWN/spine/pkg/input"

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	echoPlatform "github.com/NARUBROWN/spine-echo-platform"
	netPlatform "github.com/NARUBROWN/spine-nethttp-platform"
	httpcontract "github.com/NARUBROWN/spine/contract/http"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
	files "github.com/NARUBROWN/spine/pkg/multipart"
	"github.com/NARUBROWN/spine/pkg/route"
	"github.com/labstack/echo/v5"
)

type nativeHooks struct {
	before func(*http.Request)
	after  func(*http.Request)
}

func platformFactory(name string, hooks nativeHooks) httpcontract.Factory {
	if name == "nethttp" {
		return netPlatform.NewFactory(netPlatform.WithMiddleware(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if hooks.before != nil {
					hooks.before(r)
				}
				if hooks.after != nil {
					defer hooks.after(r)
				}
				next.ServeHTTP(w, r)
			})
		}))
	}
	return echoPlatform.NewFactory(echoPlatform.WithEcho(func(e *echo.Echo) {
		e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c *echo.Context) error {
				if hooks.before != nil {
					hooks.before(c.Request())
				}
				if hooks.after != nil {
					defer hooks.after(c.Request())
				}
				return next(c)
			}
		})
	}))
}

func prepare(t *testing.T, app spine.App, options boot.HTTPOptions) spine.PreparedApplication {
	t.Helper()
	p, err := app.Prepare(t.Context(), boot.Options{HTTP: &options})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return p
}

// managedServer는 httptest의 외부 실행 대신 플랫폼의 실제 Serve 경로를 사용합니다.
func managedServer(t *testing.T, p spine.PreparedApplication) (string, *http.Client) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- p.HTTPAdapter().Serve(listener) }()
	client := &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(func() {
		client.CloseIdleConnections()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.Close(ctx); err != nil {
			t.Error(err)
		}
		select {
		case err := <-done:
			if !errors.Is(err, httpcontract.ErrServerClosed) {
				t.Errorf("Serve 종료: %v", err)
			}
		case <-ctx.Done():
			t.Error("Serve 종료 시간 초과")
		}
	})
	return "http://" + listener.Addr().String(), client
}

type bodyDTO struct {
	Name  string `json:"name" xml:"name"`
	Count int    `json:"count" xml:"count"`
	Maybe *int   `json:"maybe" xml:"maybe"`
}
type formDTO struct {
	Title string   `form:"title" json:"title"`
	Items []string `form:"items" json:"items"`
}
type pathDTO struct {
	ID string `path:"id"`
}
type queryDTO struct {
	Tags []string `query:"tags"`
	Page *int     `query:"page"`
}
type headerDTO struct {
	Host string `header:"Host"`
}
type requiredDTO struct {
	Name *string `json:"name" validate:"required"`
}
type inputController struct{ calls atomic.Int32 }

func (c *inputController) Body(dto httpinput.Body[bodyDTO]) httpx.Response[bodyDTO] {
	c.calls.Add(1)
	return httpx.Response[bodyDTO]{Body: *dto}
}
func (c *inputController) Form(dto httpinput.Body[formDTO]) httpx.Response[formDTO] {
	c.calls.Add(1)
	return httpx.Response[formDTO]{Body: *dto}
}
func (c *inputController) Inspect(path httpinput.Path[pathDTO], query httpinput.Query[queryDTO], header httpinput.Header[headerDTO]) httpx.Response[map[string]any] {
	c.calls.Add(1)
	return httpx.Response[map[string]any]{Body: map[string]any{"id": path.ID, "tags": query.Tags, "page": query.Page, "host": header.Host}}
}
func (c *inputController) Required(dto httpinput.Body[requiredDTO]) httpx.Response[requiredDTO] {
	c.calls.Add(1)
	return httpx.Response[requiredDTO]{Body: *dto}
}

type inputValidator struct{ calls atomic.Int32 }

func (v *inputValidator) Validate(_ context.Context, dto any) error {
	v.calls.Add(1)
	if value, ok := dto.(*requiredDTO); ok && value.Name == nil {
		return validation.NewError(validation.FieldError{Field: "/name", Rule: "required"})
	}
	return nil
}

type result struct {
	Status             int
	Body               string
	Calls, Validations int32
}

func canonical(body []byte) string {
	var value any
	if json.Unmarshal(body, &value) == nil {
		normalized, _ := json.Marshal(value)
		return string(normalized)
	}
	return string(body)
}

func TestDTOResultsMatchAcrossRealServers(t *testing.T) {
	baseline := map[string]result{}
	for _, engine := range []string{"echo", "nethttp"} {
		t.Run(engine, func(t *testing.T) {
			controller, validator := &inputController{}, &inputValidator{}
			app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{})), spine.WithValidator(validator))
			app.Constructor(func() *inputController { return controller })
			app.Route("POST", "/body", (*inputController).Body)
			app.Route("POST", "/form", (*inputController).Form)
			app.Route("POST", "/required", (*inputController).Required)
			app.Route("GET", "/inspect/:id", (*inputController).Inspect)
			url, client := managedServer(t, prepare(t, app, boot.HTTPOptions{}))
			for _, tc := range []struct {
				name, path, media, body string
				status                  int
				want                    string
				calls, validations      int32
			}{
				{"json", "/body", "application/json", `{"name":"body","count":2}`, 200, `{"count":2,"maybe":null,"name":"body"}`, 1, 1},
				{"nullable-pointer", "/body", "application/json", `{"maybe":null}`, 200, `{"count":0,"maybe":null,"name":""}`, 1, 1},
				{"scalar-null", "/body", "application/json", `{"count":null}`, 400, "", 0, 0},
				{"root-null", "/body", "application/json", `null`, 400, "", 0, 0},
				{"invalid-json", "/body", "application/json", `{"name":`, 400, "", 0, 0},
				{"unsupported-media", "/body", "text/plain", `body`, 415, "", 0, 0},
				{"xml", "/body", "application/xml", `<body><name>xml</name><count>3</count></body>`, 200, `{"count":3,"maybe":null,"name":"xml"}`, 1, 1},
				{"body-only-form", "/form?title=query", "application/x-www-form-urlencoded", `title=body&items=a%2Cb&items=c`, 200, `{"items":["a,b","c"],"title":"body"}`, 1, 1},
				{"invalid-urlencoded", "/form", "application/x-www-form-urlencoded", `title=%ZZ`, 400, "", 0, 0},
				{"invalid-multipart", "/form", "multipart/form-data; boundary=missing", `broken`, 400, "", 0, 0},
				{"raw-query-and-path", "/inspect/a%252Fb?tags=a%2Cb,c&tags=d&page=2", "", "", 200, `{"host":"api.example:8443","id":"a%2Fb","page":2,"tags":["a,b","c","d"]}`, 1, 3},
				{"encoded-slash-value", "/inspect/a%2Fb?tags=a%2Cb,c&tags=d&page=2", "", "", 200, `{"host":"api.example:8443","id":"a/b","page":2,"tags":["a,b","c","d"]}`, 1, 3},
				{"invalid-query", "/inspect/id?page=invalid", "", "", 400, "", 0, 1},
				{"validation-error", "/required", "application/json", `{}`, 400, "", 0, 1},
				{"validation-success", "/required", "application/json", `{"name":"ok"}`, 200, `{"name":"ok"}`, 1, 1},
			} {
				t.Run(tc.name, func(t *testing.T) {
					beforeCalls, beforeValidation := controller.calls.Load(), validator.calls.Load()
					method := "POST"
					if strings.HasPrefix(tc.path, "/inspect/") {
						method = "GET"
					}
					r, _ := http.NewRequest(method, url+tc.path, strings.NewReader(tc.body))
					r.Host = "api.example:8443"
					r.Header.Set("Content-Type", tc.media)
					r.Header.Set("X-Forwarded-Host", "ignored.example")
					response, err := client.Do(r)
					if err != nil {
						t.Fatal(err)
					}
					body, err := io.ReadAll(response.Body)
					_ = response.Body.Close()
					if err != nil {
						t.Fatal(err)
					}
					got := result{response.StatusCode, canonical(body), controller.calls.Load() - beforeCalls, validator.calls.Load() - beforeValidation}
					if got.Status != tc.status || got.Calls != tc.calls || got.Validations != tc.validations || (tc.want != "" && got.Body != tc.want) {
						t.Fatalf("결과: %+v", got)
					}
					if engine == "echo" {
						baseline[tc.name] = got
					} else if !reflect.DeepEqual(got, baseline[tc.name]) {
						t.Fatalf("플랫폼 결과 차이: nethttp=%+v echo=%+v", got, baseline[tc.name])
					}
				})
			}
		})
	}
}

type uploadController struct {
	t       *testing.T
	path    string
	order   []string
	failure string
}

func (c *uploadController) FormFirst(form httpinput.Body[formDTO], upload files.UploadedFiles) httpx.Response[string] {
	return c.handle(form, upload)
}
func (c *uploadController) FilesFirst(upload files.UploadedFiles, form httpinput.Body[formDTO]) httpx.Response[string] {
	return c.handle(form, upload)
}
func (c *uploadController) handle(form httpinput.Body[formDTO], upload files.UploadedFiles) httpx.Response[string] {
	c.order = append(c.order, "controller")
	if form.Title != "body" || len(upload.Files) != 1 {
		c.t.Errorf("폼과 파일: %+v %+v", form, upload)
		return httpx.Response[string]{}
	}
	f, err := upload.Files[0].Open()
	if err != nil {
		c.t.Error(err)
		return httpx.Response[string]{}
	}
	defer f.Close()
	disk, ok := f.(*os.File)
	if !ok {
		c.t.Error("메모리 예산을 넘은 파일은 디스크에 있어야 합니다")
	} else {
		c.path = disk.Name()
	}
	data, err := io.ReadAll(f)
	if err != nil || string(data) != strings.Repeat("payload", 100) {
		c.t.Errorf("파일 내용: %v", err)
	}
	if c.failure == "panic" {
		panic("controller panic")
	}
	return httpx.Response[string]{Body: form.Title}
}

type uploadInterceptor struct {
	controller *uploadController
	reject     bool
}

func (i *uploadInterceptor) PreHandle(core.ExecutionContext, core.HandlerMeta) error {
	i.controller.order = append(i.controller.order, "pre")
	if i.reject {
		return httperr.Unauthorized("authentication required")
	}
	return nil
}
func (*uploadInterceptor) PostHandle(core.ExecutionContext, core.HandlerMeta) {}
func (i *uploadInterceptor) BeforeResponse(core.ExecutionContext, core.HandlerMeta, error) error {
	i.controller.order = append(i.controller.order, "before")
	if i.controller.failure == "before" {
		return errors.New("before response failed")
	}
	return nil
}
func (i *uploadInterceptor) AfterCompletion(ctx core.ExecutionContext, _ core.HandlerMeta, _ error) {
	c := i.controller
	c.order = append(c.order, "after")
	if c.path != "" {
		if _, err := os.Stat(c.path); err != nil {
			c.t.Errorf("완료 훅의 파일 수명: %v", err)
		}
		form, err := ctx.(core.HTTPRequest).MultipartForm()
		if err != nil || form.Value["title"][0] != "body" {
			c.t.Errorf("완료 훅의 파싱 결과: %v", err)
		}
	}
	if c.failure == "after" {
		panic("completion panic")
	}
}

func multipartPayload(t *testing.T) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("title", "body"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("files", "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, strings.Repeat("payload", 100)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func TestMultipartOrderAndLifetimeOnBothPlatforms(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		for _, first := range []bool{true, false} {
			for _, failure := range []string{"", "panic", "before", "after"} {
				t.Run(engine+"/"+map[bool]string{true: "form-first", false: "files-first"}[first]+"/"+failure, func(t *testing.T) {
					c := &uploadController{t: t, failure: failure}
					done := make(chan struct{})
					app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{after: func(*http.Request) {
						defer close(done)
						if c.path != "" {
							if _, err := os.Stat(c.path); !errors.Is(err, os.ErrNotExist) {
								t.Errorf("Execute 반환 시 임시 파일이 남았습니다: %v", err)
							}
						}
					}})))
					app.Constructor(func() *uploadController { return c })
					interceptor := &uploadInterceptor{controller: c}
					if first {
						app.Route("POST", "/upload", (*uploadController).FormFirst, route.WithInterceptors(interceptor))
					} else {
						app.Route("POST", "/upload", (*uploadController).FilesFirst, route.WithInterceptors(interceptor))
					}
					url, client := managedServer(t, prepare(t, app, boot.HTTPOptions{MultipartMemoryBytes: 1}))
					body, media := multipartPayload(t)
					response, err := client.Post(url+"/upload?title=query", media, bytes.NewReader(body))
					if err != nil {
						t.Fatal(err)
					}
					_, _ = io.Copy(io.Discard, response.Body)
					_ = response.Body.Close()
					waitSignal(t, done)
					want := 200
					if failure == "panic" || failure == "before" {
						want = 500
					}
					if response.StatusCode != want || strings.Join(c.order, ",") != "pre,controller,before,after" || c.path == "" {
						t.Fatalf("상태/순서/파일: %d %v %q", response.StatusCode, c.order, c.path)
					}
				})
			}
		}
	}
}

type countingBody struct {
	io.ReadCloser
	reads *atomic.Int32
}

func (b countingBody) Read(p []byte) (int, error) { b.reads.Add(1); return b.ReadCloser.Read(p) }

func TestAuthenticationDoesNotReadBodyOverTCP(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		for _, global := range []bool{false, true} {
			t.Run(engine+"/"+map[bool]string{true: "global", false: "route"}[global], func(t *testing.T) {
				var reads atomic.Int32
				done := make(chan struct{})
				c := &uploadController{t: t}
				app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{before: func(r *http.Request) { r.Body = countingBody{r.Body, &reads} }, after: func(*http.Request) { close(done) }})))
				app.Constructor(func() *uploadController { return c })
				interceptor := &uploadInterceptor{controller: c, reject: true}
				if global {
					app.Interceptor(interceptor)
					app.Route("POST", "/upload", (*uploadController).FormFirst)
				} else {
					app.Route("POST", "/upload", (*uploadController).FormFirst, route.WithInterceptors(interceptor))
				}
				url, client := managedServer(t, prepare(t, app, boot.HTTPOptions{}))
				body, media := multipartPayload(t)
				response, err := client.Post(url+"/upload", media, bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
				waitSignal(t, done)
				if response.StatusCode != 401 || reads.Load() != 0 || strings.Contains(strings.Join(c.order, ","), "controller") {
					t.Fatalf("인증/읽기/순서: %d/%d/%v", response.StatusCode, reads.Load(), c.order)
				}
			})
		}
	}
}

func TestBodyLimitAndMultipartErrorsOverTCP(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		for _, chunked := range []bool{false, true} {
			for _, multipartBody := range []bool{false, true} {
				t.Run(engine+"/"+map[bool]string{true: "chunked", false: "fixed"}[chunked]+"/"+map[bool]string{true: "multipart", false: "json"}[multipartBody], func(t *testing.T) {
					c := &inputController{}
					app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{})))
					app.Constructor(func() *inputController { return c })
					path, media, body := "/body", "application/json", []byte(`{"name":"`+strings.Repeat("x", 100)+`"}`)
					if multipartBody {
						app.Route("POST", "/form", (*inputController).Form)
						path = "/form"
						body, media = multipartPayload(t)
					} else {
						app.Route("POST", "/body", (*inputController).Body)
					}
					url, client := managedServer(t, prepare(t, app, boot.HTTPOptions{MaxBodyBytes: 32}))
					r, _ := http.NewRequest("POST", url+path, bytes.NewReader(body))
					r.Header.Set("Content-Type", media)
					if chunked {
						r.ContentLength = -1
					}
					response, err := client.Do(r)
					if err != nil {
						t.Fatal(err)
					}
					data, err := io.ReadAll(response.Body)
					_ = response.Body.Close()
					var failure struct {
						In string `json:"in"`
					}
					_ = json.Unmarshal(data, &failure)
					if err != nil || response.StatusCode != 413 || failure.In != "body" || c.calls.Load() != 0 {
						t.Fatalf("크기 제한: %d %s %v", response.StatusCode, data, err)
					}
				})
			}
		}
	}
}

func waitSignal(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("요청 완료 시간 초과")
	}
}

// 이 보조 함수는 Prepared의 선택 handler와 외부 서버 수명도 검증합니다.
func externalServer(t *testing.T, p spine.PreparedApplication) *httptest.Server {
	t.Helper()
	handler, err := spine.NetHTTPHandler(p)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}
