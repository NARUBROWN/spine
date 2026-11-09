package openapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	playground "github.com/NARUBROWN/spine-validator-playground"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
	left "github.com/NARUBROWN/spine/test/openapi/testdata/left"
	right "github.com/NARUBROWN/spine/test/openapi/testdata/right"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

type RegressionFormFields struct {
	ID string `form:"id" validate:"required"`
}
type regressionForm struct{ RegressionFormFields }
type regressionNamedForm struct {
	Inner RegressionFormFields `json:"-"`
}
type regressionQuery struct {
	ID string `query:"id"`
}
type regressionShape struct {
	Array [2]int `json:"array"`
	Bytes []byte `json:"bytes"`
}
type regressionArrayRules struct {
	Array [2]int `json:"array" validate:"len=2,dive,gte=1"`
}
type regressionExcludedQuery struct {
	ID     string `query:"id"`
	Hidden string `query:"-" validate:"required"`
}
type regressionExcludedHeader struct {
	ID     string `header:"X-ID"`
	Hidden string `header:"-" validate:"required"`
}
type regressionController struct{}

func (*regressionController) Form(f *regressionForm, q *regressionQuery) httpx.Response[map[string]string] {
	return httpx.Response[map[string]string]{Body: map[string]string{"body": f.ID, "query": q.ID}}
}
func (*regressionController) Named(f *regressionNamedForm) httpx.Response[string] {
	return httpx.Response[string]{Body: f.Inner.ID}
}
func (*regressionController) Shape(f *regressionShape) httpx.Response[regressionShape] {
	return httpx.Response[regressionShape]{Body: *f}
}
func (*regressionController) ArrayRules(f *regressionArrayRules) httpx.Response[regressionArrayRules] {
	return httpx.Response[regressionArrayRules]{Body: *f}
}
func (*regressionController) Left() httpx.Response[struct{ Value left.Item }] {
	return httpx.Response[struct{ Value left.Item }]{}
}
func (*regressionController) Right() httpx.Response[struct{ Value right.Item }] {
	return httpx.Response[struct{ Value right.Item }]{}
}
func (*regressionController) ExcludedQuery(*regressionExcludedQuery)   {}
func (*regressionController) ExcludedHeader(*regressionExcludedHeader) {}

func regressionApplication(t *testing.T, configured bool) (http.Handler, map[string]any) {
	t.Helper()
	opts := []spine.Option{platform.Option()}
	if configured {
		v, e := playground.New()
		if e != nil {
			t.Fatal(e)
		}
		opts = append(opts, spine.WithValidator(v))
	}
	app := spine.New(opts...)
	app.Constructor(func() *regressionController { return &regressionController{} })
	app.Route("POST", "/form", (*regressionController).Form)
	app.Route("POST", "/named", (*regressionController).Named)
	app.Route("POST", "/shape", (*regressionController).Shape)
	app.Route("POST", "/array-rules", (*regressionController).ArrayRules)
	app.Route("GET", "/left", (*regressionController).Left)
	app.Route("GET", "/right", (*regressionController).Right)
	p, e := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true}}})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = p.Close(context.Background()) })
	w := serve(mustHTTPHandler(t, p), "GET", "/openapi.json", "", nil)
	doc, e := jsonschema.UnmarshalJSON(w.Body)
	if e != nil {
		t.Fatal(e)
	}
	return mustHTTPHandler(t, p), doc.(map[string]any)
}

func TestFormBodyIsIsolatedFromURLAndIncludesEmbeddedFieldsInDescription(t *testing.T) {
	for _, configured := range []bool{false, true} {
		h, doc := regressionApplication(t, configured)
		w := serve(h, "POST", "/form?id=from-query", "application/x-www-form-urlencoded", strings.NewReader("other=body"))
		want := 200
		if configured {
			want = 400
		}
		if w.Code != want || strings.Contains(w.Body.String(), `"body":"from-query"`) {
			t.Fatalf("폼 본문과 쿼리를 분리해야 합니다: %d %s", w.Code, w.Body)
		}
		w = serve(h, "POST", "/form?id=from-query", "application/x-www-form-urlencoded", strings.NewReader("id=from-body"))
		var result map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &result)
		if w.Code != 200 || result["body"] != "from-body" || result["query"] != "from-query" {
			t.Fatalf("출처별 값이 달라야 합니다: %d %s", w.Code, w.Body)
		}
		w = serve(h, "POST", "/named", "application/x-www-form-urlencoded", strings.NewReader("id=from-body"))
		if w.Code != 200 || w.Body.String() != "from-body" {
			t.Fatalf("중첩 폼 바인딩: %d %s", w.Code, w.Body)
		}
		if !configured {
			continue
		}
		for _, path := range []string{"/form", "/named"} {
			schema := compile(t, doc, contentSchema(operation(doc, path, "post")["requestBody"].(map[string]any)["content"], "application/x-www-form-urlencoded"))
			if schema.Validate(map[string]any{"id": "value"}) != nil || schema.Validate(map[string]any{}) == nil {
				t.Fatalf("펼친 폼의 필수 필드를 설명해야 합니다: %s", path)
			}
		}
	}
}

func TestJSONRequestAndResponseShapesMatchGoDecoding(t *testing.T) {
	h, doc := regressionApplication(t, true)
	for _, test := range []struct {
		path, body string
		valid      bool
	}{
		{"/shape", `{"array":[1],"bytes":[1,2]}`, true},
		{"/shape", `{"array":[1,2,3],"bytes":"AQI="}`, true},
		{"/shape", `{"array":[1,2,null],"bytes":null}`, true},
		{"/shape", `{"array":[],"bytes":[256]}`, false},
		{"/shape", `{"array":[null],"bytes":null}`, false},
		{"/array-rules", `{"array":[1,2,0]}`, true},
		{"/array-rules", `{"array":[1,2,null]}`, true},
		{"/array-rules", `{"array":[1]}`, false},
		{"/array-rules", `{"array":[1,0]}`, false},
	} {
		schema := compile(t, doc, contentSchema(operation(doc, test.path, "post")["requestBody"].(map[string]any)["content"], "application/json"))
		input, e := jsonschema.UnmarshalJSON(strings.NewReader(test.body))
		if e != nil {
			t.Fatal(e)
		}
		if (schema.Validate(input) == nil) != test.valid {
			t.Fatalf("요청 명세 판정: %s", test.body)
		}
		w := serve(h, "POST", test.path, "application/json", strings.NewReader(test.body))
		if (w.Code == 200) != test.valid {
			t.Fatalf("실제 바인딩 판정: %d %s", w.Code, w.Body)
		}
		assertJSONResponse(t, doc, test.path, "post", w)
	}
}

func TestAnonymousResponseSchemasDoNotMergeSameNamedPackageTypes(t *testing.T) {
	h, doc := regressionApplication(t, true)
	for _, path := range []string{"/left", "/right"} {
		assertJSONResponse(t, doc, path, "get", serve(h, "GET", path, "", nil))
	}
}

func TestExcludedRequiredInputFieldsFailDuringBootWithOrWithoutOpenAPI(t *testing.T) {
	v, e := playground.New()
	if e != nil {
		t.Fatal(e)
	}
	for _, handler := range []any{(*regressionController).ExcludedQuery, (*regressionController).ExcludedHeader} {
		for _, configured := range []bool{false, true} {
			for _, enabled := range []bool{false, true} {
				opts := []spine.Option{platform.Option()}
				if configured {
					opts = append(opts, spine.WithValidator(v))
				}
				app := spine.New(opts...)
				app.Route("GET", "/excluded", handler)
				app.Constructor(func() *regressionController {
					panic("설정 충돌보다 먼저 생성자를 실행하면 안 됩니다")
				})
				options := boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: enabled}}}
				check := func(e error) {
					t.Helper()
					if !configured {
						if e != nil {
							t.Fatal(e)
						}
						return
					}
					var failure *boot.ConfigError
					if !errors.As(e, &failure) || len(failure.Issues) != 1 || failure.Issues[0].Code != "HTTP_INPUT_VALIDATION_CONFLICT" {
						t.Fatalf("제외 필드의 필수 검증 충돌: %v", e)
					}
				}
				check(app.Validate(options))
				_, e := spine.DescribeHTTP(app, options)
				check(e)
				if configured {
					_, e = app.Prepare(t.Context(), options)
					check(e)
					check(app.RunContext(t.Context(), options))
				}
			}
		}
	}
}
