package openapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	playground "github.com/NARUBROWN/spine-validator-playground"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
)

type requiredCodecText string

func (v *requiredCodecText) UnmarshalText(data []byte) error {
	*v = requiredCodecText(data)
	return nil
}

type requiredQuotedBody struct {
	Plain    int `json:"plain" validate:"required"`
	Quoted   int `json:"quoted,string" validate:"required"`
	Optional int `json:"optional,string"`
}

type requiredCodecQuery struct {
	Value    requiredCodecText `query:"value" validate:"required"`
	Optional requiredCodecText `query:"optional"`
}

type requiredCodecHeader struct {
	Value    requiredCodecText `header:"X-Value" validate:"required"`
	Optional requiredCodecText `header:"X-Optional"`
}

type requiredCodecController struct{}

func (*requiredCodecController) Constructor() *requiredCodecController {
	return &requiredCodecController{}
}

func (*requiredCodecController) RegisterRoutes(r *api.Registry) {
	r.POST("/quoted", func(_ input.Body[requiredQuotedBody]) httpx.Response[string] {
		return httpx.Response[string]{Body: "ok"}
	})
	r.GET("/text-query", func(_ input.Query[requiredCodecQuery]) httpx.Response[string] {
		return httpx.Response[string]{Body: "ok"}
	})
	r.GET("/text-header", func(_ input.Header[requiredCodecHeader]) httpx.Response[string] {
		return httpx.Response[string]{Body: "ok"}
	})
}

func TestRequiredFieldsSurviveUnresolvedWireConstraints(t *testing.T) {
	v, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	app := spine.New(platform.Option(), spine.WithValidator(v))
	app.Constructor(constructor.Controller(&requiredCodecController{}))
	prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Close(context.Background()) })
	h := mustHTTPHandler(t, prepared)
	w := serve(h, http.MethodGet, "/openapi.json", "", nil)
	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil || w.Code != http.StatusOK {
		t.Fatalf("OpenAPI: status=%d err=%v body=%s", w.Code, err, w.Body)
	}

	t.Run("quoted-body-schema", func(t *testing.T) {
		body := operation(doc, "/quoted", "post")["requestBody"].(map[string]any)
		schema := contentSchema(body["content"], "application/json").(map[string]any)
		ref := strings.TrimPrefix(schema["$ref"].(string), "#/components/schemas/")
		object := doc["components"].(map[string]any)["schemas"].(map[string]any)[ref].(map[string]any)
		if !reflect.DeepEqual(object["required"], []any{"plain", "quoted"}) {
			t.Fatalf("quoted required and optional fields: %#v", object)
		}
		properties := object["properties"].(map[string]any)
		if properties["quoted"].(map[string]any)["type"] != "string" {
			t.Fatalf("quoted number must keep its string wire type: %#v", properties["quoted"])
		}
		compiled := compile(t, doc, schema)
		if compiled.Validate(map[string]any{"plain": 1.0}) == nil {
			t.Fatal("schema accepted a missing required quoted field")
		}
		if err := compiled.Validate(map[string]any{"plain": 1.0, "quoted": "2"}); err != nil {
			t.Fatalf("schema rejected valid quoted input without optional field: %v", err)
		}
	})

	for _, route := range []string{"/text-query", "/text-header"} {
		t.Run(route+"-parameters", func(t *testing.T) {
			parameters := operation(doc, route, "get")["parameters"].([]any)
			if len(parameters) != 2 {
				t.Fatalf("required and optional parameters must both be documented: %#v", parameters)
			}
			for _, raw := range parameters {
				parameter := raw.(map[string]any)
				name := parameter["name"].(string)
				want := name == "value" || name == "X-Value"
				if parameter["required"] != want {
					t.Fatalf("custom decoder field required: %#v", parameter)
				}
			}
		})
	}

	for _, tc := range []struct {
		name, method, target, body, header string
		status                             int
	}{
		{"missing-quoted", "POST", "/quoted", `{"plain":1}`, "", 400},
		{"valid-quoted", "POST", "/quoted", `{"plain":1,"quoted":"2"}`, "", 200},
		{"missing-text-query", "GET", "/text-query", "", "", 400},
		{"valid-text-query", "GET", "/text-query?value=bound", "", "", 200},
		{"missing-text-header", "GET", "/text-header", "", "", 400},
		{"valid-text-header", "GET", "/text-header", "", "bound", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			if tc.header != "" {
				req.Header.Set("X-Value", tc.header)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("runtime validation: status=%d want=%d body=%s", w.Code, tc.status, w.Body)
			}
		})
	}
}
