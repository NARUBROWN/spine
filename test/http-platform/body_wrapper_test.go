package httpplatform_test

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	playground "github.com/NARUBROWN/spine-validator-playground"
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
	r.POST("/body", func(body input.Body[wrapperFormBody]) (httpx.Response[string], error) {
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

type bodyMediaController struct{}

func (*bodyMediaController) RegisterRoutes(r *api.Registry) {
	r.POST("/form", func(body input.Body[formOnlyBody]) httpx.Response[string] {
		return httpx.Response[string]{Body: body.Value}
	})
	r.POST("/mixed", func(body input.Body[wrapperFormBody]) httpx.Response[string] {
		return httpx.Response[string]{Body: body.Name}
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
					}
				}
			}
			if counts["/form"] != 2 || counts["/mixed"] != 5 {
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
