package openapi_test

import httpinput "github.com/NARUBROWN/spine/pkg/input"

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	playground "github.com/NARUBROWN/spine-validator-playground"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

type sourceBody struct {
	ID string `json:"id" xml:"xml_id" validate:"required"`
}
type sourceQuery struct {
	Count *int `json:"json_count" query:"count" validate:"required"`
}
type sourceHeader struct {
	ID string `json:"json_id" header:"X-ID" validate:"required"`
}
type sourceForm struct {
	ID string `json:"json_id" form:"form_id" validate:"required"`
}
type sourceMatch struct {
	Match []string `header:"If-Match"`
}
type sourceEncoding struct {
	Encoding string `header:"Accept-Encoding"`
}
type bareBody struct{ Name string }
type sourcePath struct {
	ID int64 `path:"id"`
}
type sourceController struct{}

func (*sourceController) Body(input httpinput.Body[sourceBody]) httpx.Response[sourceBody] {
	return httpx.Response[sourceBody]{Body: *input}
}
func (*sourceController) Query(input httpinput.Query[sourceQuery]) httpx.Response[sourceQuery] {
	return httpx.Response[sourceQuery]{Body: *input}
}
func (*sourceController) Header(input httpinput.Header[sourceHeader]) httpx.Response[sourceHeader] {
	return httpx.Response[sourceHeader]{Body: *input}
}
func (*sourceController) Form(input httpinput.Body[sourceForm]) httpx.Response[sourceForm] {
	return httpx.Response[sourceForm]{Body: *input}
}
func (*sourceController) Match(input httpinput.Header[sourceMatch]) httpx.Response[[]string] {
	return httpx.Response[[]string]{Body: input.Match}
}
func (*sourceController) Encoding(input httpinput.Header[sourceEncoding]) httpx.Response[string] {
	return httpx.Response[string]{Body: input.Encoding}
}
func (*sourceController) Plain(_ httpinput.Path[sourcePath], input httpinput.Body[bareBody]) httpx.Response[bareBody] {
	return httpx.Response[bareBody]{Body: *input}
}

func sourceApplication(t *testing.T, configured bool) http.Handler {
	t.Helper()
	options := []spine.Option{platform.Option()}
	if configured {
		v, err := playground.New()
		if err != nil {
			t.Fatal(err)
		}
		options = append(options, spine.WithValidator(v))
	}
	app := spine.New(options...)
	app.Constructor(func() *sourceController { return &sourceController{} })
	app.Route("POST", "/body", (*sourceController).Body)
	app.Route("GET", "/query", (*sourceController).Query)
	app.Route("GET", "/header", (*sourceController).Header)
	app.Route("POST", "/form", (*sourceController).Form)
	app.Route("GET", "/match", (*sourceController).Match)
	app.Route("GET", "/encoding", (*sourceController).Encoding)
	app.Route("POST", "/plain/:id", (*sourceController).Plain)
	prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Close(context.Background()) })
	return mustHTTPHandler(t, prepared)
}

func TestHTTPValidationUsesBoundSourceAndWireNames(t *testing.T) {
	h := sourceApplication(t, true)
	for _, test := range []struct{ method, target, media, body, source, field string }{
		{"POST", "/body", "application/json", "{}", "body", "/id"},
		{"POST", "/body", "application/xml", "<request/>", "body", "/xml_id"},
		{"GET", "/query", "", "", "query", "/count"},
		{"GET", "/header", "", "", "header", "/X-Id"},
		{"POST", "/form", "application/x-www-form-urlencoded", "", "body", "/form_id"},
		{"POST", "/form", "application/json", "{}", "body", "/json_id"},
	} {
		w := serve(h, test.method, test.target, test.media, strings.NewReader(test.body))
		var result struct {
			Fields []struct{ In, Field, Rule string } `json:"fields"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 400 || len(result.Fields) != 1 || result.Fields[0].In != test.source || result.Fields[0].Field != test.field {
			t.Fatalf("입력 출처와 실제 키: target=%s status=%d body=%s err=%v", test.target, w.Code, w.Body, err)
		}
	}
	for _, test := range []struct{ method, target, body, source string }{
		{"POST", "/body", `{"id":null}`, "body"},
		{"GET", "/query?count=not-an-int", "", "query"},
	} {
		w := serve(h, test.method, test.target, "application/json", strings.NewReader(test.body))
		var result map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &result)
		if w.Code != 400 || result["in"] != test.source || result["fields"] != nil {
			t.Fatalf("바인딩 오류와 검증 오류를 구분해야 합니다: %d %s", w.Code, w.Body)
		}
	}
}

func TestXMLRequestMediaAndWireNamesMatchOpenAPI(t *testing.T) {
	h := sourceApplication(t, true)
	document := serve(h, "GET", "/openapi.json", "", nil)
	var spec struct {
		Paths map[string]map[string]struct {
			RequestBody struct {
				Content map[string]struct {
					Schema struct {
						Ref string `json:"$ref"`
					}
				}
			}
		}
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage
				Required   []string
			}
		}
	}
	if err := json.Unmarshal(document.Body.Bytes(), &spec); err != nil || document.Code != http.StatusOK {
		t.Fatalf("OpenAPI: %d %v %s", document.Code, err, document.Body)
	}
	for _, media := range []string{"application/xml", "text/xml"} {
		result := serve(h, "POST", "/body", media, strings.NewReader("<request><xml_id>bound</xml_id></request>"))
		if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), `"id":"bound"`) {
			t.Fatalf("%s: %d %s", media, result.Code, result.Body)
		}
		ref := spec.Paths["/body"]["post"].RequestBody.Content[media].Schema.Ref
		schema := spec.Components.Schemas[strings.TrimPrefix(ref, "#/components/schemas/")]
		if schema.Properties["xml_id"] == nil || schema.Properties["id"] != nil || !reflect.DeepEqual(schema.Required, []string{"xml_id"}) {
			t.Fatalf("%s schema %s: %+v", media, ref, schema)
		}
	}
}

func TestHeaderMultiplicityAndQuotedListsUseTheSameRulesWithOrWithoutValidator(t *testing.T) {
	for _, configured := range []bool{false, true} {
		h := sourceApplication(t, configured)
		server := httptest.NewServer(h)
		t.Cleanup(server.Close)
		for _, test := range []struct {
			target, name string
			values       []string
			status       int
			want         any
		}{
			{"/header", "X-ID", []string{"same", "same"}, 400, nil},
			{"/encoding", "Accept-Encoding", []string{"gzip", "br"}, 200, "gzip, br"},
			{"/match", "If-Match", []string{`"a,b", "c"`}, 200, []string{`"a,b"`, `"c"`}},
			{"/match", "If-Match", []string{`W/"a,b"`, `"c"`}, 200, []string{`W/"a,b"`, `"c"`}},
			{"/match", "If-Match", []string{`"unclosed`}, 400, nil},
		} {
			req, err := http.NewRequest("GET", server.URL+test.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header[test.name] = test.values
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != test.status {
				t.Fatalf("validator=%v header=%s status=%d body=%s", configured, test.name, response.StatusCode, data)
			}
			if test.want == nil {
				continue
			}
			if want, ok := test.want.(string); ok {
				if string(data) != want {
					t.Fatalf("전체 목록 문자열을 보존해야 합니다: %s", data)
				}
			} else {
				var got []string
				if err := json.Unmarshal(data, &got); err != nil || !reflect.DeepEqual(got, test.want) {
					t.Fatalf("헤더 목록 원소 경계가 달라졌습니다: %s %v", data, err)
				}
			}
		}
	}
}

func TestNoValidatorKeepsUntaggedBindingAndOnlyDocumentsRoutes(t *testing.T) {
	h := sourceApplication(t, false)
	w := serve(h, "POST", "/plain/1", "application/json", strings.NewReader(`{"Name":"Kim"}`))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"Name":"Kim"`) {
		t.Fatalf("태그 없는 본문 바인딩을 유지해야 합니다: %d %s", w.Code, w.Body)
	}
	document := serve(h, "GET", "/openapi.json", "", nil)
	var doc map[string]any
	if err := json.Unmarshal(document.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["components"] != nil {
		t.Fatal("validator 없이 모델을 명세하면 안 됩니다")
	}
	for _, item := range doc["paths"].(map[string]any) {
		for _, raw := range item.(map[string]any) {
			op := raw.(map[string]any)
			if op["requestBody"] != nil {
				t.Fatal("validator 없이 본문을 명세하면 안 됩니다")
			}
			if parameters, ok := op["parameters"].([]any); ok {
				for _, raw := range parameters {
					parameter := raw.(map[string]any)
					if parameter["in"] != "path" || len(parameter["schema"].(map[string]any)) != 0 {
						t.Fatal("경로 변수 외의 상세 입력이나 타입을 추측하면 안 됩니다")
					}
				}
			}
			responses := op["responses"].(map[string]any)
			if len(responses) != 1 || responses["default"].(map[string]any)["content"] != nil {
				t.Fatal("응답 스키마나 상태 코드를 추측하면 안 됩니다")
			}
		}
	}
	assertDocumentStructure(t, doc)
}
