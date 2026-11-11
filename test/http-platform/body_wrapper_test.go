package httpplatform_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	playground "github.com/NARUBROWN/spine-validator-playground"
	event "github.com/NARUBROWN/spine/contract/event"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
	handlerspine "github.com/NARUBROWN/spine/pkg/spine"
)

type wrapperFormBody struct {
	Name string                `json:"name" form:"name"`
	File *multipart.FileHeader `json:"-" form:"file"`
}
type bodyWrapperController struct{}

func (*bodyWrapperController) RegisterRoutes(r *api.Registry) {
	r.POST("/body", func(body input.Body[wrapperFormBody], ctx handlerspine.Ctx) (httpx.Response[string], error) {
		result := body.Name
		if body.File != nil {
			file, err := body.File.Open()
			if err != nil {
				return httpx.Response[string]{}, err
			}
			defer file.Close()
			data, err := io.ReadAll(file)
			if err != nil {
				return httpx.Response[string]{}, err
			}
			result += ":" + body.File.Filename + ":" + string(data)
			body.File.Filename = "changed"
			body.File.Header.Set("X-Changed", "yes")
			request, _ := ctx.HTTPRequestContext()
			form, err := request.MultipartForm()
			if err != nil {
				return httpx.Response[string]{}, err
			}
			if file := form.File["file"][0]; file.Filename == "changed" || file.Header.Get("X-Changed") != "" {
				return httpx.Response[string]{}, fmt.Errorf("multipart metadata escaped the DTO")
			}
		}
		return httpx.Response[string]{Body: result}, nil
	})
}
func TestBodyWrapperJSONFormAndMultipart(t *testing.T) {
	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	if err := writer.WriteField("name", "Alice"); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, adapter := range []string{"echo", "nethttp"} {
		t.Run(adapter, func(t *testing.T) {
			app := spine.New(spine.WithHTTPAdapter(platformFactory(adapter, nativeHooks{})))
			app.Constructor(func() *bodyWrapperController { return &bodyWrapperController{} })
			prepared := prepare(t, app, boot.HTTPOptions{})
			base, client := managedServer(t, prepared)
			for _, tc := range []struct{ media, body, want string }{
				{"application/json", `{"name":"Alice"}`, "Alice"},
				{"application/x-www-form-urlencoded", "name=Alice", "Alice"},
				{writer.FormDataContentType(), upload.String(), "Alice:hello.txt:hello"},
			} {
				t.Run(tc.media, func(t *testing.T) {
					response, err := client.Post(base+"/body?name=wrong", tc.media, strings.NewReader(tc.body))
					if err != nil {
						t.Fatal(err)
					}
					data, err := io.ReadAll(response.Body)
					response.Body.Close()
					if err != nil {
						t.Fatal(err)
					}
					if response.StatusCode != http.StatusOK || strings.TrimSpace(string(data)) != tc.want {
						t.Fatalf("%d %s", response.StatusCode, data)
					}
				})
			}
		})
	}
}

type formOnlyBody struct {
	Value string `form:"value" json:"-" xml:"-" validate:"required"`
}

type unavailableBody struct {
	Value string `json:"-" xml:"-" validate:"required"`
}

type unsupportedFormBody struct {
	Value map[string]int `form:"value"`
}

func (*unsupportedFormBody) UnmarshalJSON([]byte) error { return nil }

type xmlMapBody struct {
	Value map[string]int `json:"-" xml:"value" validate:"required"`
}

type marshalMapKey struct{ Value string }

func (k marshalMapKey) MarshalText() ([]byte, error) { return []byte(k.Value), nil }

type marshalMapBody struct {
	Value map[marshalMapKey]int `json:"value" xml:"-" validate:"required"`
}

type unmarshalMapKey struct{ Value string }

func (k *unmarshalMapKey) UnmarshalText(value []byte) error {
	k.Value = string(value)
	return nil
}

type unmarshalMapBody struct {
	Value map[unmarshalMapKey]int `json:"value" validate:"required"`
}

type optionalXMLChild struct {
	Name string `json:"-" xml:"name" validate:"required"`
}

type optionalChildBody struct {
	Child *optionalXMLChild `json:"child" xml:"child"`
}

type bodyMediaController struct{}

func (*bodyMediaController) RegisterRoutes(r *api.Registry) {
	r.POST("/form", func(body input.Body[formOnlyBody]) httpx.Response[string] {
		return httpx.Response[string]{Body: body.Value}
	})
	r.POST("/mixed", func(body input.Body[wrapperFormBody]) httpx.Response[string] {
		return httpx.Response[string]{Body: body.Name}
	})
	r.POST("/map", func(body input.Body[unmarshalMapBody]) httpx.Response[int] {
		return httpx.Response[int]{Body: body.Value[unmarshalMapKey{Value: "key"}]}
	})
	r.POST("/optional", func(body input.Body[optionalChildBody]) httpx.Response[bool] {
		return httpx.Response[bool]{Body: body.Child != nil}
	})
	r.POST("/publish", func(ctx handlerspine.Ctx) httpx.Response[string] {
		publish.Event(ctx.ExecutionContext().Context(), testDomainEvent{})
		return httpx.Response[string]{Body: "queued"}
	})
	r.POST("/publish-written", func(ctx handlerspine.Ctx) error {
		publish.Event(ctx.ExecutionContext().Context(), testDomainEvent{})
		response, _ := ctx.HTTPResponseContext()
		return response.WriteString(http.StatusOK, "queued")
	})
}

type testDomainEvent struct{}

func (testDomainEvent) Name() string          { return "test.created" }
func (testDomainEvent) OccurredAt() time.Time { return time.Unix(1, 0) }

type invalidMediaController struct {
	bind any
}

func (c *invalidMediaController) RegisterRoutes(r *api.Registry) {
	r.POST("/body", c.bind)
}

func TestBodyMediaDescriptionAndBindingAgreeOverTCP(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		t.Run(engine, func(t *testing.T) {
			validator, err := playground.New()
			if err != nil {
				t.Fatal(err)
			}
			app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{})), spine.WithValidator(validator))
			app.Constructor(func() *bodyMediaController { return &bodyMediaController{} })
			prepared := prepare(t, app, boot.HTTPOptions{})
			desc, err := spine.DescribePreparedHTTP(prepared)
			if err != nil {
				t.Fatal(err)
			}
			for _, diagnostic := range desc.Diagnostics {
				if diagnostic.Code == "MAP_KEY_UNRESOLVED" {
					t.Errorf("TextUnmarshaler map 키의 명세를 확인하지 못했습니다: %v", diagnostic)
				}
			}
			counts := map[string]int{}
			for _, route := range desc.Routes {
				for _, param := range route.Inputs {
					if param.Binding != "body" {
						continue
					}
					counts[route.Path] = len(param.Content)
					if route.Path == "/form" && (param.Content["application/x-www-form-urlencoded"] == nil || param.Content["multipart/form-data"] == nil) {
						t.Errorf("폼 입력 명세가 다릅니다: %v", param.Content)
					}
					if route.Path == "/mixed" && (param.Content["application/json"] == nil || param.Content["application/x-www-form-urlencoded"] == nil) {
						t.Errorf("선택 파일 때문에 일반 입력 명세가 누락됐습니다: %v", param.Content)
						encoded := string(param.Content["application/x-www-form-urlencoded"])
						var reference struct {
							Ref string `json:"$ref"`
						}
						if err := json.Unmarshal([]byte(encoded), &reference); err != nil {
							t.Fatal(err)
						}
						key := strings.TrimPrefix(reference.Ref, "#/components/schemas/")
						if strings.Contains(string(desc.Schemas[key]), `"file"`) {
							t.Errorf("URL 인코딩 폼 명세에 파일이 포함됐습니다: %s", desc.Schemas[key])
						}
					}
				}
			}
			if counts["/form"] != 2 || counts["/mixed"] != 5 || counts["/map"] != 1 {
				t.Fatalf("본문 형식 개수가 다릅니다: %v", counts)
			}
			base, client := managedServer(t, prepared)
			for _, tc := range []struct {
				path, media, body string
				status            int
			}{
				{"/form", "application/json", `{"value":"ignored"}`, http.StatusUnsupportedMediaType},
				{"/form", "application/xml", "<body/>", http.StatusUnsupportedMediaType},
				{"/form", "application/x-www-form-urlencoded", "value=valid", http.StatusOK},
				{"/form", "application/x-www-form-urlencoded", "value=first&value=second", http.StatusBadRequest},
				{"/form", "text/plain", "", http.StatusUnsupportedMediaType},
				{"/map", "application/json", `{"value":{"key":4}}`, http.StatusOK},
				{"/map", "application/xml", "<body><value>4</value></body>", http.StatusUnsupportedMediaType},
				{"/optional", "application/json", `{}`, http.StatusOK},
				{"/optional", "application/json", `{"child":{}}`, http.StatusBadRequest},
				{"/optional", "application/xml", "<body><child><name>ok</name></child></body>", http.StatusOK},
				{"/mixed", "application/json", `{"name":"valid"}`, http.StatusOK},
				{"/mixed", "application/x-www-form-urlencoded", "name=valid", http.StatusOK},
				{"/mixed", "application/xml", "garbage<body><Name>valid</Name></body>", http.StatusBadRequest},
				{"/publish", "", "", http.StatusServiceUnavailable},
				{"/publish-written", "", "", http.StatusServiceUnavailable},
			} {
				response, err := client.Post(base+tc.path, tc.media, strings.NewReader(tc.body))
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if err != nil || response.StatusCode != tc.status {
					t.Errorf("%s %s 상태: %d, 기대값: %d, 응답: %s, 오류: %v", tc.path, tc.media, response.StatusCode, tc.status, data, err)
				}
			}
			for _, tc := range []struct {
				bind any
				code string
			}{
				{func(input.Body[unavailableBody]) {}, "HTTP_INPUT_VALIDATION_CONFLICT"},
				{func(input.Body[xmlMapBody]) {}, "HTTP_INPUT_VALIDATION_CONFLICT"},
				{func(input.Body[marshalMapBody]) {}, "HTTP_INPUT_VALIDATION_CONFLICT"},
				{func(input.Body[unsupportedFormBody]) {}, "HTTP_INPUT_CONTRACT_INVALID"},
			} {
				bad := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{})), spine.WithValidator(validator))
				bad.Constructor(func() *invalidMediaController { return &invalidMediaController{bind: tc.bind} })
				_, err := bad.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{}})
				if err == nil || !strings.Contains(err.Error(), tc.code) {
					t.Errorf("준비 단계에서 %s 오류를 기대했습니다: %v", tc.code, err)
				}
			}
		})
	}
}

type latePublishInterceptor struct {
	phase string
	fail  bool
}

func (latePublishInterceptor) PreHandle(handlerspine.Ctx, core.HandlerMeta) error { return nil }
func (i latePublishInterceptor) PostHandle(ctx handlerspine.Ctx, _ core.HandlerMeta) {
	if i.phase == "post" {
		publish.Event(ctx.ExecutionContext().Context(), testDomainEvent{})
	}
}
func (i latePublishInterceptor) BeforeResponse(ctx handlerspine.Ctx, _ core.HandlerMeta, err error) error {
	if i.phase == "before" && err == nil {
		publish.Event(ctx.ExecutionContext().Context(), testDomainEvent{})
		if i.fail {
			return fmt.Errorf("finalization failed")
		}
	}
	return nil
}
func (latePublishInterceptor) AfterCompletion(handlerspine.Ctx, core.HandlerMeta, error) {}

type latePublishFactory struct{ published *atomic.Int32 }

func (f latePublishFactory) Capabilities() event.Capabilities {
	return event.Capabilities{ContractVersion: event.ContractVersion, Publish: true}
}
func (f latePublishFactory) NewPublisher(context.Context) (event.Publisher, error) { return f, nil }
func (latePublishFactory) NewReader(context.Context, event.Subscription) (event.Reader, error) {
	return nil, fmt.Errorf("consumer is unavailable")
}
func (f latePublishFactory) Publish(context.Context, event.Message) error {
	f.published.Add(1)
	return nil
}
func (latePublishFactory) Close(context.Context) error { return nil }

func TestLateInterceptorEventsOverTCP(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		for _, tc := range []struct {
			phase            string
			configured, fail bool
			status           int
			published        int32
		}{
			{"post", true, false, http.StatusOK, 1},
			{"post", false, false, http.StatusServiceUnavailable, 0},
			{"before", true, false, http.StatusOK, 1},
			{"before", false, false, http.StatusServiceUnavailable, 0},
			{"before", true, true, http.StatusInternalServerError, 0},
		} {
			t.Run(fmt.Sprintf("%s/%s/%t/%t", engine, tc.phase, tc.configured, tc.fail), func(t *testing.T) {
				var published atomic.Int32
				options := []spine.Option{spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{}))}
				if tc.configured {
					options = append(options, spine.WithEventAdapter("test", latePublishFactory{published: &published}))
				}
				app := spine.New(options...)
				app.Interceptor(latePublishInterceptor{phase: tc.phase, fail: tc.fail})
				app.Constructor(func() *bodyWrapperController { return &bodyWrapperController{} })
				base, client := managedServer(t, prepare(t, app, boot.HTTPOptions{}))
				response, err := client.Post(base+"/body", "application/json", strings.NewReader(`{"name":"test"}`))
				if err != nil {
					t.Fatal(err)
				}
				_ = response.Body.Close()
				if response.StatusCode != tc.status || published.Load() != tc.published {
					t.Fatalf("상태 %d와 발행 %d건, 기대값 %d와 %d건", response.StatusCode, published.Load(), tc.status, tc.published)
				}
			})
		}
	}
}
