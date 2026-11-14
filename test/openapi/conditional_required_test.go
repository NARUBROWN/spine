package openapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	playground "github.com/NARUBROWN/spine-validator-playground"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
)

type conditionalBody struct {
	Operation string `json:"operation" form:"action" validate:"required,oneof=review undo"`
	ID        string `json:"id" form:"record_id" validate:"required_if=Operation undo"`
}

type conditionalValidator struct {
	*playground.Validator
}

func (v conditionalValidator) DescribeSchema(t reflect.Type) (validation.SchemaDescription, error) {
	description, err := v.Validator.DescribeSchema(t)
	if err == nil && len(description.ConditionalRequired) == 0 {
		// 공개된 기존 검증기와도 코어의 새 명세 계약을 검사합니다.
		description.ConditionalRequired = []validation.ConditionalRequired{{
			Index:  []int{1},
			When:   []validation.FieldCondition{{Index: []int{0}, Value: json.RawMessage(`"undo"`)}},
			Schema: json.RawMessage(`{"minLength":1}`),
		}}
	}
	return description, err
}

type conditionalController struct{}

func (*conditionalController) Constructor() *conditionalController {
	return &conditionalController{}
}

func (*conditionalController) RegisterRoutes(r *api.Registry) {
	r.POST("/actions", func(_ input.Body[conditionalBody]) httpx.Response[string] {
		return httpx.Response[string]{Body: "ok"}
	})
}

func TestConditionalRequiredRequestSchema(t *testing.T) {
	validator, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	app := spine.New(platform.Option(), spine.WithValidator(conditionalValidator{validator}))
	app.Constructor(constructor.Controller(&conditionalController{}))
	prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Close(context.Background()) })
	handler := mustHTTPHandler(t, prepared)
	response := serve(handler, http.MethodGet, "/openapi.json", "", nil)
	var document map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &document); response.Code != http.StatusOK || err != nil {
		t.Fatalf("OpenAPI: status=%d err=%v", response.Code, err)
	}
	body := operation(document, "/actions", "post")["requestBody"].(map[string]any)
	for _, test := range []struct {
		media, operation, id, missing, empty, valid, skipped string
	}{
		{"application/json", "operation", "id", `{"operation":"undo"}`, `{"operation":"undo","id":""}`, `{"operation":"undo","id":"1"}`, `{"operation":"review"}`},
		{"application/x-www-form-urlencoded", "action", "record_id", "action=undo", "action=undo&record_id=", "action=undo&record_id=1", "action=review"},
	} {
		t.Run(test.media, func(t *testing.T) {
			compiled := compile(t, document, contentSchema(body["content"], test.media))
			for _, row := range []struct {
				wire  string
				value map[string]any
				code  int
			}{
				{test.missing, map[string]any{test.operation: "undo"}, http.StatusBadRequest},
				{test.empty, map[string]any{test.operation: "undo", test.id: ""}, http.StatusBadRequest},
				{test.valid, map[string]any{test.operation: "undo", test.id: "1"}, http.StatusOK},
				{test.skipped, map[string]any{test.operation: "review"}, http.StatusOK},
			} {
				actual := serve(handler, http.MethodPost, "/actions", test.media, strings.NewReader(row.wire))
				if actual.Code != row.code {
					t.Fatalf("%s: expected %d, got %d: %s", row.wire, row.code, actual.Code, actual.Body)
				}
				if err := compiled.Validate(row.value); (err == nil) != (row.code == http.StatusOK) {
					t.Fatalf("%s: schema and HTTP result differ: %v", row.wire, err)
				}
			}
		})
	}
}
