package openapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	playground "github.com/NARUBROWN/spine-validator-playground"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/header"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

type payload struct {
	Email    string         `json:"email" validate:"required,email"`
	Age      int            `json:"age" validate:"gte=18,lte=120"`
	Flag     bool           `json:"flag" validate:"required"`
	Zero     *int           `json:"zero" validate:"required"`
	Optional int            `json:"optional" validate:"omitempty,gte=18"`
	Name     string         `json:"name" validate:"omitempty,min=2"`
	Pointer  *int           `json:"pointer" validate:"omitnil,gte=18"`
	Tags     []string       `json:"tags" validate:"omitempty,min=1,dive,required"`
	Labels   map[string]int `json:"labels" validate:"dive,gte=1"`
}
type formDTO struct {
	Name string `json:"name" form:"display_name" validate:"required,min=2"`
	Age  int    `json:"age" form:"age" validate:"gte=18"`
}
type queryDTO struct {
	Age  int      `json:"age" query:"min_age" validate:"gte=18"`
	Tags []string `json:"tags" query:"tag" validate:"omitempty,dive,required"`
}
type uploadDTO struct {
	Avatar      *multipart.FileHeader   `json:"avatar" form:"avatar" validate:"required"`
	Attachments []*multipart.FileHeader `json:"attachments" form:"attachments"`
}
type searchResult struct {
	Age  int      `json:"age"`
	Tags []string `json:"tags"`
	ID   string   `json:"id"`
}
type uploadResult struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}
type controller struct{}

func (*controller) Create(value *payload) httpx.Response[payload] {
	return httpx.Response[payload]{Body: *value}
}
func (*controller) Form(value *formDTO) httpx.Response[formDTO] {
	return httpx.Response[formDTO]{Body: *value}
}
func (*controller) Search(values *queryDTO, headers header.Values) httpx.Response[searchResult] {
	return httpx.Response[searchResult]{Body: searchResult{Age: values.Age, Tags: values.Tags, ID: headers.Get("X-Request-ID")}}
}
func (*controller) Upload(value *uploadDTO) httpx.Response[uploadResult] {
	return httpx.Response[uploadResult]{Body: uploadResult{Name: value.Avatar.Filename, Count: len(value.Attachments)}}
}
func (*controller) Nullable() httpx.Response[*payload] { return httpx.Response[*payload]{} }
func (*controller) String() httpx.Response[string] {
	return httpx.Response[string]{Body: "hello", Options: httpx.ResponseOptions{Status: 201}}
}
func (*controller) Binary() httpx.Binary {
	return httpx.Binary{Data: []byte{1, 2, 3}, ContentType: "image/png"}
}
func (*controller) Conflict() error { return &httperr.HTTPError{Status: 409, Message: "conflict"} }

func application(t *testing.T) (http.Handler, map[string]any) {
	t.Helper()
	v, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	app := spine.New(platform.Option(), spine.WithValidator(v))
	app.Constructor(func() *controller { return &controller{} })
	app.Route("POST", "/users", (*controller).Create)
	app.Route("POST", "/form", (*controller).Form)
	app.Route("GET", "/search", (*controller).Search)
	app.Route("POST", "/upload", (*controller).Upload)
	app.Route("GET", "/nullable", (*controller).Nullable)
	app.Route("GET", "/string", (*controller).String)
	app.Route("GET", "/binary", (*controller).Binary)
	app.Route("GET", "/conflict", (*controller).Conflict)
	prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := prepared.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	h := prepared.Handler()
	w := serve(h, "GET", "/openapi.json", "", nil)
	if w.Code != 200 {
		t.Fatalf("문서: %d %s", w.Code, w.Body)
	}
	doc, err := jsonschema.UnmarshalJSON(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	return h, doc.(map[string]any)
}

func serve(h http.Handler, method, path, media string, body io.Reader) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, body)
	if media != "" {
		req.Header.Set("Content-Type", media)
	}
	h.ServeHTTP(w, req)
	return w
}
func operation(doc map[string]any, path, method string) map[string]any {
	return doc["paths"].(map[string]any)[path].(map[string]any)[method].(map[string]any)
}
func contentSchema(content any, media string) any {
	return content.(map[string]any)[media].(map[string]any)["schema"]
}
func responseSchema(doc map[string]any, path, method, status, media string) any {
	responses := operation(doc, path, method)["responses"].(map[string]any)
	response, exists := responses[status]
	if !exists {
		response = responses["default"]
	}
	return contentSchema(response.(map[string]any)["content"], media)
}
func compile(t *testing.T, doc map[string]any, schema any) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	root := map[string]any{"components": doc["components"], "allOf": []any{schema}}
	if err := compiler.AddResource("https://spine.test/schema", root); err != nil {
		t.Fatal(err)
	}
	result, err := compiler.Compile("https://spine.test/schema")
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func assertJSONResponse(t *testing.T, doc map[string]any, path, method string, w *httptest.ResponseRecorder) {
	t.Helper()
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	schema := compile(t, doc, responseSchema(doc, path, method, fmt.Sprint(w.Code), "application/json"))
	if err := schema.Validate(value); err != nil {
		t.Fatalf("실제 응답과 명세 불일치: %d %s\n%v", w.Code, w.Body, err)
	}
}

func TestOpenAPIDocumentConformsToOfficialStructure(t *testing.T) {
	_, doc := application(t)
	file, err := os.Open("testdata/openapi-3.1-schema.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	structure, err := jsonschema.UnmarshalJSON(file)
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	const location = "https://spec.openapis.org/oas/3.1/schema/2022-10-07"
	if err := compiler.AddResource(location, structure); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(doc); err != nil {
		t.Fatal(err)
	}
}

func TestRequestSchemaAndActualValidationAgreeOn35BoundaryInputs(t *testing.T) {
	h, doc := application(t)
	if operation(doc, "/users", "post")["requestBody"].(map[string]any)["required"] != true {
		t.Fatal("필수 DTO 본문을 설명해야 합니다")
	}
	empty := serve(h, "POST", "/users", "application/json", nil)
	if empty.Code != 400 {
		t.Fatalf("필수 본문 누락: %d %s", empty.Code, empty.Body)
	}
	requestSchema := compile(t, doc, contentSchema(operation(doc, "/users", "post")["requestBody"].(map[string]any)["content"], "application/json"))
	base := map[string]any{"email": "alice@example.com", "age": 18, "flag": true, "zero": 0}
	cases := []map[string]any{base}
	for _, key := range []string{"email", "age", "flag", "zero"} {
		value := clone(base)
		delete(value, key)
		cases = append(cases, value)
	}
	for _, test := range []struct {
		key    string
		values []any
	}{
		{"email", []any{"", "invalid", "ok@example.com"}}, {"age", []any{0, 17, 18, 120, 121}},
		{"flag", []any{false, true}}, {"zero", []any{nil, 0, 1}}, {"optional", []any{0, 1, 18}},
		{"name", []any{"", "a", "ab"}}, {"pointer", []any{nil, 0, 18}},
		{"tags", []any{nil, []string{}, []string{""}, []string{"ok"}}},
		{"labels", []any{nil, map[string]int{}, map[string]int{"x": 0}, map[string]int{"x": 1}}},
	} {
		for _, item := range test.values {
			value := clone(base)
			value[test.key] = item
			cases = append(cases, value)
		}
	}
	if len(cases) != 35 {
		t.Fatalf("경계 입력 수: %d", len(cases))
	}
	for index, sample := range cases {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			encoded, err := json.Marshal(sample)
			if err != nil {
				t.Fatal(err)
			}
			value, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
			if err != nil {
				t.Fatal(err)
			}
			valid := requestSchema.Validate(value) == nil
			w := serve(h, "POST", "/users", "application/json", bytes.NewReader(encoded))
			if (w.Code == 200) != valid || w.Code != 200 && w.Code != 400 {
				t.Fatalf("요청 명세와 실제 판정 불일치: schema=%v status=%d input=%s body=%s", valid, w.Code, encoded, w.Body)
			}
			assertJSONResponse(t, doc, "/users", "post", w)
		})
	}
}
func clone(source map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range source {
		result[key] = value
	}
	return result
}

func TestExistingQueryDTOAndHeaderValuesKeepPlatformBinding(t *testing.T) {
	h, doc := application(t)
	codes := map[string]bool{}
	for _, raw := range doc["x-spine-diagnostics"].([]any) {
		codes[raw.(map[string]any)["code"].(string)] = true
	}
	if !codes["DTO_QUERY_BINDING_UNRESOLVED"] || !codes["DYNAMIC_PARAMETERS_UNRESOLVED"] {
		t.Fatal("플랫폼 쿼리 바인딩과 동적 헤더의 한계를 설명해야 합니다")
	}
	for _, test := range []struct {
		age, id string
		status  int
	}{{"18", "request-1", 200}, {"17", "request-1", 400}, {"invalid", "request-1", 400}, {"18", "", 200}} {
		req := httptest.NewRequest("GET", "/search?min_age="+url.QueryEscape(test.age)+"&tag=a&tag=b", nil)
		if test.id != "" {
			req.Header.Set("X-Request-ID", test.id)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != test.status {
			t.Fatalf("기존 DTO와 헤더 바인딩: %d %s", w.Code, w.Body)
		}
		assertJSONResponse(t, doc, "/search", "get", w)
		if w.Code == 200 && (!strings.Contains(w.Body.String(), `"age":18`) || !strings.Contains(w.Body.String(), `"tags":["a","b"]`) || !strings.Contains(w.Body.String(), `"id":"`+test.id+`"`)) {
			t.Fatalf("기존 입력의 직접 접근을 유지해야 합니다: %s", w.Body)
		}
	}
}

func TestExistingFormUploadsAndReturnTypesWithoutDeclarations(t *testing.T) {
	h, doc := application(t)
	if len(operation(doc, "/upload", "post")["requestBody"].(map[string]any)["content"].(map[string]any)) != 1 {
		t.Fatal("파일 DTO는 multipart 미디어로만 기술해야 합니다")
	}
	for _, name := range []string{"a", "alice"} {
		model := map[string]any{"display_name": name, "age": 18}
		schema := compile(t, doc, contentSchema(operation(doc, "/form", "post")["requestBody"].(map[string]any)["content"], "application/x-www-form-urlencoded"))
		w := serve(h, "POST", "/form", "application/x-www-form-urlencoded", strings.NewReader(url.Values{"display_name": {name}, "age": {"18"}}.Encode()))
		if (w.Code == 200) != (schema.Validate(model) == nil) {
			t.Fatalf("폼 명세와 실제 판정: %d %s", w.Code, w.Body)
		}
		assertJSONResponse(t, doc, "/form", "post", w)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, field := range []string{"avatar", "attachments", "attachments"} {
		part, err := writer.CreateFormFile(field, field+".png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte{1, 2, 3}); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	model := map[string]any{"avatar": "binary file", "attachments": []any{"file 1", "file 2"}}
	schema := compile(t, doc, contentSchema(operation(doc, "/upload", "post")["requestBody"].(map[string]any)["content"], "multipart/form-data"))
	if err := schema.Validate(model); err != nil {
		t.Fatal(err)
	}
	w := serve(h, "POST", "/upload", writer.FormDataContentType(), &body)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"count":2`) {
		t.Fatalf("파일 바인딩: %d %s", w.Code, w.Body)
	}
	assertJSONResponse(t, doc, "/upload", "post", w)
	for _, path := range []string{"/nullable", "/conflict"} {
		w := serve(h, "GET", path, "", nil)
		assertJSONResponse(t, doc, path, "get", w)
	}
	w = serve(h, "GET", "/string", "", nil)
	if w.Code != 201 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") || compile(t, doc, responseSchema(doc, "/string", "get", "default", "text/plain")).Validate(w.Body.String()) != nil {
		t.Fatalf("문자열 응답: %d %s", w.Code, w.Body)
	}
	w = serve(h, "GET", "/binary", "", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || !bytes.Equal(w.Body.Bytes(), []byte{1, 2, 3}) {
		t.Fatalf("바이너리 응답: %d %s", w.Code, w.Body)
	}
	if err := compile(t, doc, responseSchema(doc, "/binary", "get", "default", "*/*")).Validate(w.Body.String()); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeStatusAndMediaAreNotGuessedFromHandlers(t *testing.T) {
	_, doc := application(t)
	for _, path := range []string{"/string", "/binary", "/conflict"} {
		responses := operation(doc, path, "get")["responses"].(map[string]any)
		if len(responses) != 1 || responses["default"] == nil {
			t.Fatalf("런타임 상태를 고정값으로 추측하면 안 됩니다: %s %+v", path, responses)
		}
	}
	content := operation(doc, "/binary", "get")["responses"].(map[string]any)["default"].(map[string]any)["content"].(map[string]any)
	if len(content) != 1 || content["*/*"] == nil {
		t.Fatalf("동적 바이너리 미디어를 추측하면 안 됩니다: %+v", content)
	}
}
