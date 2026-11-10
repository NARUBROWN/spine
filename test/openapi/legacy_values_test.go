package openapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/header"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/query"
	handlerctx "github.com/NARUBROWN/spine/pkg/spine"
)

type legacyContextKey struct{}
type legacyValuesController struct{}
type legacyValuesResult struct {
	Tag     string `json:"tag"`
	Count   int64  `json:"count"`
	Enabled bool   `json:"enabled"`
	Header  string `json:"header"`
	Empty   bool   `json:"empty"`
	Host    string `json:"host"`
	Page    int    `json:"page"`
	Size    int    `json:"size"`
	Context string `json:"context"`
	User    string `json:"user"`
}

func (*legacyValuesController) Get(ctx context.Context, view handlerctx.Ctx, q query.Values, h header.Values, page query.Pagination) httpx.Response[legacyValuesResult] {
	user, _ := view.Get("user")
	return httpx.Response[legacyValuesResult]{Body: legacyValuesResult{
		Tag: q.String("tag"), Count: q.Int("count", 7), Enabled: q.GetBoolByKey("enabled", false),
		Header: h.Get("X-Test"), Empty: h.Has("X-Empty"), Host: h.Get("Host"), Page: page.Page, Size: page.Size,
		Context: ctx.Value(legacyContextKey{}).(string), User: user.(string),
	}}
}

type legacyValuesValidator struct {
	calls     atomic.Int32
	describes atomic.Int32
}

func (v *legacyValuesValidator) Validate(context.Context, any) error {
	v.calls.Add(1)
	return nil
}
func (v *legacyValuesValidator) DescribeSchema(reflect.Type) (validation.SchemaDescription, error) {
	v.describes.Add(1)
	panic("기존 조회의 입력 키를 추측하면 안 됩니다")
}

type legacyValuesInterceptor struct{}

func (*legacyValuesInterceptor) PreHandle(ctx core.ExecutionContext, _ core.HandlerMeta) error {
	ctx.Set("user", "stored-user")
	return nil
}
func (*legacyValuesInterceptor) PostHandle(core.ExecutionContext, core.HandlerMeta) {}
func (*legacyValuesInterceptor) BeforeResponse(core.ExecutionContext, core.HandlerMeta, error) error {
	return nil
}
func (*legacyValuesInterceptor) AfterCompletion(core.ExecutionContext, core.HandlerMeta, error) {}

func TestDeprecatedValuesWorkOverHTTPAndLeaveDynamicInputsUnresolved(t *testing.T) {
	for _, configured := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			v := &legacyValuesValidator{}
			opts := []spine.Option{platform.Option()}
			if configured {
				opts = append(opts, spine.WithValidator(v))
			}
			app := spine.New(opts...)
			app.Constructor(func() *legacyValuesController { return &legacyValuesController{} })
			app.Interceptor(&legacyValuesInterceptor{})
			app.Route("GET", "/legacy", (*legacyValuesController).Get)
			prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: enabled}}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := prepared.Close(context.Background()); err != nil {
					t.Error(err)
				}
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mustHTTPHandler(t, prepared).ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), legacyContextKey{}, "request-context")))
			}))
			t.Cleanup(server.Close)
			for _, test := range []struct {
				pagination string
				page, size int
			}{{"page=3&size=40", 3, 40}, {"page=bad&size=bad", 1, 20}, {"page=-1&size=0", -1, 0}} {
				req, err := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/legacy?tag=a,b&tag=c&count=bad&enabled=YeS&"+test.pagination, nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Host = "legacy.example:8443"
				req.Header.Add("X-Test", "first")
				req.Header.Add("X-Test", "second")
				req.Header.Set("X-Empty", "")
				response, err := server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				var got legacyValuesResult
				err = json.NewDecoder(response.Body).Decode(&got)
				_ = response.Body.Close()
				want := legacyValuesResult{Tag: "a,b", Count: 7, Enabled: true, Header: "first", Empty: true, Host: "legacy.example:8443", Page: test.page, Size: test.size, Context: "request-context", User: "stored-user"}
				if err != nil || response.StatusCode != 200 || got != want {
					t.Fatalf("validator=%v enabled=%v: status=%d got=%+v err=%v", configured, enabled, response.StatusCode, got, err)
				}
			}
			if v.calls.Load() != 0 || v.describes.Load() != 0 {
				t.Fatal("기존 조회 타입에 DTO 검증이나 스키마 추정을 추가하면 안 됩니다")
			}
			resource := serve(mustHTTPHandler(t, prepared), "GET", "/openapi.json", "", nil)
			if !enabled {
				if resource.Code != 404 {
					t.Fatalf("명세 비활성화: %d", resource.Code)
				}
				continue
			}
			var doc map[string]any
			if err := json.Unmarshal(resource.Body.Bytes(), &doc); err != nil || resource.Code != 200 {
				t.Fatalf("명세: %s %v", resource.Body, err)
			}
			assertDocumentStructure(t, doc)
			op := operation(doc, "/legacy", "get")
			if !configured {
				if op["parameters"] != nil {
					t.Fatal("검증기 없이는 경로만 설명해야 합니다")
				}
				continue
			}
			parameters := op["parameters"].([]any)
			if len(parameters) != 2 || parameters[0].(map[string]any)["name"] != "page" || parameters[1].(map[string]any)["name"] != "size" || op["requestBody"] != nil {
				t.Fatalf("기존 조회 키는 추측하지 않고 Pagination만 설명해야 합니다: %+v", op)
			}
			codes := map[string]bool{}
			for _, diagnostic := range doc["x-spine-diagnostics"].([]any) {
				codes[diagnostic.(map[string]any)["code"].(string)] = true
			}
			if !codes["LEGACY_QUERY_VALUES_UNRESOLVED"] || !codes["LEGACY_HEADER_VALUES_UNRESOLVED"] {
				t.Fatalf("기존 동적 입력의 명세 한계를 표시해야 합니다: %+v", codes)
			}
		}
	}
}
