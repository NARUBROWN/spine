package openapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	playground "github.com/NARUBROWN/spine-validator-playground"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/path"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// 필드 순서는 라우트 순서와 의도적으로 다르게 둡니다.
type fivePath struct {
	Revision  int    `path:"revision" validate:"gte=1"`
	Enabled   bool   `path:"enabled"`
	ItemID    int64  `path:"item_id" validate:"gte=0"`
	OrgID     string `path:"org_id"`
	ProjectID uint8  `path:"project_id"`
}
type pathBody struct {
	ID string `json:"item_id"`
}
type pathQuery struct {
	ID string `query:"item_id"`
}
type pathHeader struct {
	ID string `header:"item_id"`
}
type pathResult struct {
	Org      string `json:"org"`
	Project  uint8  `json:"project"`
	Item     int64  `json:"item"`
	Revision int    `json:"revision"`
	Enabled  bool   `json:"enabled"`
	Body     string `json:"body"`
	Query    string `json:"query"`
	Header   string `json:"header"`
}
type optionalPathPointer struct {
	ID *int `path:"id" validate:"omitnil,gte=0"`
}
type unusedPathKey struct {
	ID int64 `path:"id"`
}
type excludedPath struct {
	ID     int    `path:"id"`
	Hidden string `path:"-" validate:"required"`
}
type PathBase struct {
	Org string `path:"org"`
}
type embeddedPath struct {
	PathBase
	ID int `path:"id"`
}
type tokenPath struct {
	Token pathAtom `path:"token"`
}
type pathAtom string

func (v *pathAtom) UnmarshalText(b []byte) error { *v = pathAtom("decoded:" + string(b)); return nil }

type pathController struct{ calls int }

func (c *pathController) Update(p *fivePath, b *pathBody, q *pathQuery, h *pathHeader) httpx.Response[pathResult] {
	c.calls++
	return httpx.Response[pathResult]{Body: pathResult{Org: p.OrgID, Project: p.ProjectID, Item: p.ItemID, Revision: p.Revision, Enabled: p.Enabled, Body: b.ID, Query: q.ID, Header: h.ID}}
}
func (c *pathController) Pointer(p *optionalPathPointer) httpx.Response[int] {
	c.calls++
	return httpx.Response[int]{Body: *p.ID}
}
func (c *pathController) Unused(p *unusedPathKey) httpx.Response[int64] {
	c.calls++
	return httpx.Response[int64]{Body: p.ID}
}
func (c *pathController) Embedded(p *embeddedPath) httpx.Response[string] {
	c.calls++
	return httpx.Response[string]{Body: p.Org}
}
func (c *pathController) Token(p *tokenPath) httpx.Response[string] {
	c.calls++
	return httpx.Response[string]{Body: string(p.Token)}
}
func (*pathController) Excluded(*excludedPath) {}
func (*pathController) Legacy(p path.Int) httpx.Response[int64] {
	return httpx.Response[int64]{Body: p.Value}
}

const fiveRoute = "/orgs/:org_id/projects/:project_id/items/:item_id/revisions/:revision/flags/:enabled"
const fiveTemplate = "/orgs/{org_id}/projects/{project_id}/items/{item_id}/revisions/{revision}/flags/{enabled}"

func pathApplication(t *testing.T, configured bool) (http.Handler, map[string]any, *pathController) {
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
	c := &pathController{}
	app.Constructor(func() *pathController { return c })
	app.Route("POST", fiveRoute, (*pathController).Update)
	app.Route("GET", "/pointer/:id", (*pathController).Pointer)
	app.Route("GET", "/unused/:org/:id", (*pathController).Unused)
	app.Route("GET", "/embedded/:org/:id", (*pathController).Embedded)
	app.Route("GET", "/token/:token", (*pathController).Token)
	app.Route("GET", "/legacy/:id", (*pathController).Legacy)
	p, e := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true}}})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = p.Close(context.Background()) })
	w := serve(p.Handler(), "GET", "/openapi.json", "", nil)
	doc, e := jsonschema.UnmarshalJSON(w.Body)
	if e != nil {
		t.Fatal(e)
	}
	return p.Handler(), doc.(map[string]any), c
}

func TestFivePathFieldsUseNamesAndStaySeparateFromOtherSources(t *testing.T) {
	for _, configured := range []bool{false, true} {
		h, doc, c := pathApplication(t, configured)
		server := httptest.NewServer(h)
		t.Cleanup(server.Close)
		req, e := http.NewRequest("POST", server.URL+"/orgs/acme/projects/255/items/0/revisions/1/flags/false?item_id=from-query", strings.NewReader(`{"item_id":"from-body"}`))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("item_id", "from-header")
		resp, e := server.Client().Do(req)
		if e != nil {
			t.Fatal(e)
		}
		var result pathResult
		e = json.NewDecoder(resp.Body).Decode(&result)
		_ = resp.Body.Close()
		want := pathResult{Org: "acme", Project: 255, Item: 0, Revision: 1, Enabled: false, Body: "from-body", Query: "from-query", Header: "from-header"}
		if e != nil || resp.StatusCode != 200 || result != want || c.calls != 1 {
			t.Fatalf("이름과 출처 기반 바인딩: got=%+v status=%d err=%v calls=%d", result, resp.StatusCode, e, c.calls)
		}
		assertDocumentStructure(t, doc)
	}
}

func TestPathFailuresHaveActualNamesAndNeverInvokeTheHandler(t *testing.T) {
	for _, configured := range []bool{false, true} {
		h, _, c := pathApplication(t, configured)
		for _, test := range []struct {
			url, field string
			rule       bool
		}{
			{"/orgs/acme/projects/256/items/1/revisions/1/flags/false", "", false},
			{"/orgs/acme/projects/1/items/invalid/revisions/1/flags/false", "", false},
			{"/orgs/acme/projects/1/items/1/revisions/1/flags/invalid", "", false},
			{"/orgs/acme/projects/1/items/1/revisions/0/flags/false", "/revision", true},
		} {
			before := c.calls
			w := serve(h, "POST", test.url, "application/json", strings.NewReader(`{}`))
			if test.rule && !configured {
				if w.Code != 200 || c.calls != before+1 {
					t.Fatalf("미등록 검증 규칙을 실행하면 안 됩니다: %d %s", w.Code, w.Body)
				}
				continue
			}
			var failure struct {
				In     string                             `json:"in"`
				Fields []struct{ In, Field, Rule string } `json:"fields"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &failure)
			if w.Code != 400 || c.calls != before {
				t.Fatalf("경로 오류는 핸들러 전에 400이어야 합니다: %d %s calls=%d", w.Code, w.Body, c.calls)
			}
			if test.rule {
				if len(failure.Fields) != 1 || failure.Fields[0].In != "path" || failure.Fields[0].Field != test.field {
					t.Fatalf("경로 검증 오류의 출처와 실제 이름: %s", w.Body)
				}
			} else if failure.In != "path" || len(failure.Fields) != 0 {
				t.Fatalf("경로 변환 오류의 출처: %s", w.Body)
			}
		}
		w := serve(h, "GET", "/pointer", "", nil)
		if w.Code != 404 {
			t.Fatalf("URL 구간 누락은 선택 필드로 해석하지 않습니다: %d %s", w.Code, w.Body)
		}
	}
}

func TestPathSchemaRequiresPresenceWithoutForcingNonZeroOrNullableValues(t *testing.T) {
	h, doc, _ := pathApplication(t, true)
	op := operation(doc, fiveTemplate, "post")
	count := 0
	for _, raw := range op["parameters"].([]any) {
		p := raw.(map[string]any)
		if p["in"] != "path" {
			continue
		}
		count++
		if p["required"] != true {
			t.Fatal("경로 존재 여부는 검증 태그와 무관하게 필수입니다")
		}
	}
	if count != 5 {
		t.Fatalf("구조체를 개별 경로 파라미터 다섯 개로 설명해야 합니다: %d", count)
	}
	for _, raw := range operation(doc, "/pointer/{id}", "get")["parameters"].([]any) {
		p := raw.(map[string]any)
		if p["in"] != "path" {
			continue
		}
		schema := compile(t, doc, p["schema"])
		if p["required"] != true || schema.Validate(nil) == nil || schema.Validate(0) != nil {
			t.Fatalf("포인터가 경로를 선택적이거나 null로 만들면 안 됩니다: %+v", p)
		}
	}
	for _, url := range []string{"/pointer/0", "/unused/org/7", "/embedded/acme/7", "/token/a%252Fb,c", "/legacy/7"} {
		w := serve(h, "GET", url, "", nil)
		if w.Code != 200 {
			t.Fatalf("경로 처리: %s %d %s", url, w.Code, w.Body)
		}
		if strings.HasPrefix(url, "/token/") && w.Body.String() != "decoded:a%2Fb,c" {
			t.Fatalf("경로 값을 다시 URL 디코딩하거나 쉼표 분할하면 안 됩니다: %s", w.Body)
		}
	}
	if !strings.Contains(mustJSON(doc), "PATH_TYPE_UNRESOLVED") {
		t.Fatal("사용하지 않는 경로 키의 타입을 추정하면 안 됩니다")
	}
	w := serve(h, "GET", "/legacy/not-an-int", "", nil)
	if w.Code != 400 || !strings.Contains(w.Body.String(), `"in":"path"`) {
		t.Fatalf("전환 중 기존 경로 오류에도 출처를 보존해야 합니다: %d %s", w.Code, w.Body)
	}
	assertJSONResponse(t, doc, "/legacy/{id}", "get", w)
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestExcludedPathValidationConflictsAtBootEvenWhenDocsAreDisabled(t *testing.T) {
	v, e := playground.New()
	if e != nil {
		t.Fatal(e)
	}
	for _, enabled := range []bool{false, true} {
		app := spine.New(platform.Option(), spine.WithValidator(v))
		app.Route("GET", "/excluded/:id", (*pathController).Excluded)
		e = app.Validate(boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: enabled}}})
		if e == nil || !strings.Contains(e.Error(), "HTTP_INPUT_VALIDATION_CONFLICT") {
			t.Fatalf("제외한 경로 필드의 필수 검증 충돌: %v", e)
		}
	}
}
