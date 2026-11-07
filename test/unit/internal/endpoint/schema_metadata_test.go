package endpoint

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/contract/description"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type examplePayload struct {
	ID      int `json:"id" example:"1" description:"user id"`
	Invalid int `json:"invalid" example:"bad"`
}

func TestFieldExamplesAndIntegerRanges(t *testing.T) {
	c := newCatalog(nil)
	value := c.structure(reflect.TypeFor[examplePayload](), "response/json")
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	for _, v := range []string{`"examples":[1]`, `"description":"user id"`, `"x-spine-type-range":true`} {
		if !strings.Contains(out, v) {
			t.Fatalf("missing %s in %s", v, out)
		}
	}
	if strconv.IntSize == 64 && !strings.Contains(out, `"format":"int64"`) {
		t.Fatal(out)
	}
	if strconv.IntSize == 32 && !strings.Contains(out, `"format":"int32"`) {
		t.Fatal(out)
	}
	if len(c.diagnostics) != 1 || c.diagnostics[0].Code != "FIELD_EXAMPLE_INVALID" {
		t.Fatalf("diagnostics: %#v", c.diagnostics)
	}
}

func TestOpenAPIPreservesMetadataInPathsOnly(t *testing.T) {
	contract := description.HTTP{PathsOnly: true, Routes: []description.Route{{Method: "GET", Path: "/users", Tags: []string{"Users"}, Summary: "Find", Description: "Find a user"}}}
	data, err := OpenAPI(contract, boot.OpenAPIOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	op := document["paths"].(map[string]any)["/users"].(map[string]any)["get"].(map[string]any)
	if op["summary"] != "Find" || op["description"] != "Find a user" || !reflect.DeepEqual(op["tags"], []any{"Users"}) {
		t.Fatalf("operation: %#v", op)
	}
}

func TestExamplesMatchWireType(t *testing.T) {
	for _, text := range []string{"01", "+1", "999999999999999999999999"} {
		if _, err := fieldExample(reflect.TypeFor[int64](), false, text); err == nil {
			t.Errorf("accepted invalid integer %q", text)
		}
	}
	if _, err := fieldExample(reflect.TypeFor[int64](), true, "not-a-number"); err == nil {
		t.Fatal("accepted invalid quoted integer")
	}
	if value, err := fieldExample(reflect.TypeFor[int64](), true, "12"); err != nil || value != "12" {
		t.Fatalf("quoted value %v: %v", value, err)
	}
}

func TestExampleConstraintConflicts(t *testing.T) {
	schema := json.RawMessage(`{"allOf":[{"exclusiveMinimum":0},{"maximum":5}]}`)
	for _, text := range []string{"0", "6"} {
		if exampleMatchesConstraints(json.Number(text), schema) {
			t.Errorf("accepted %s", text)
		}
	}
	if !exampleMatchesConstraints(json.Number("1"), schema) {
		t.Fatal("rejected valid example")
	}
	if exampleMatchesConstraints("가나", json.RawMessage(`{"minLength":3}`)) {
		t.Fatal("accepted short string")
	}
}

func TestExampleDescribedConstraints(t *testing.T) {
	tests := []struct {
		name        string
		value       any
		schema      string
		want, known bool
	}{
		{"valid-email", "user@example.com", `{"format":"email"}`, true, false},
		{"invalid-email", "not-an-email", `{"format":"email"}`, false, true},
		{"display-name", "Name <user@example.com>", `{"format":"email"}`, true, false},
		{"valid-uuid", "550e8400-e29b-41d4-a716-446655440000", `{"format":"uuid"}`, true, false},
		{"invalid-uri", "not a url", `{"format":"uri"}`, false, true},
		{"enum-numbers", json.Number("2"), `{"enum":[1,2,3]}`, true, true},
		{"enum-invalid", json.Number("4"), `{"enum":[1,2,3]}`, false, true},
		{"required-false", false, `{"const":true}`, false, true},
		{"not-zero", json.Number("0"), `{"not":{"const":0}}`, false, true},
		{"anyof-zero", json.Number("0"), `{"anyOf":[{"const":0},{"minimum":1}]}`, true, true},
		{"unknown-keyword", "x", `{"someFutureRule":true}`, true, false},
		{"pattern-dialect", "abc", `{"pattern":"^a"}`, true, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, known := checkExampleConstraints(test.value, json.RawMessage(test.schema))
			if got != test.want || known != test.known {
				t.Fatalf("got=(%v,%v), want=(%v,%v)", got, known, test.want, test.known)
			}
		})
	}
}

type exampleSchemaValidator struct{}

func (exampleSchemaValidator) Validate(context.Context, any) error {
	panic("example validation must not execute application validators")
}
func (exampleSchemaValidator) DescribeSchema(reflect.Type) (validation.SchemaDescription, error) {
	return validation.SchemaDescription{Fields: []validation.FieldSchema{
		{Index: []int{0}, Schema: json.RawMessage(`{"format":"email"}`)},
		{Index: []int{1}, Schema: json.RawMessage(`{"format":"email"}`)},
		{Index: []int{2}, Schema: json.RawMessage(`{"enum":["reader","writer"]}`)},
	}}, nil
}
func TestFieldExamplesUseSchemaWithoutExecutingValidator(t *testing.T) {
	type payload struct {
		Bad     string `json:"bad" example:"not-an-email"`
		Unknown string `json:"unknown" example:"user@example.com"`
		Role    string `json:"role" example:"admin"`
	}
	c := newCatalog(exampleSchemaValidator{})
	schema := c.structure(reflect.TypeFor[payload](), "request/json").(map[string]any)
	properties := schema["properties"].(map[string]any)
	bad := properties["bad"].(map[string]any)
	good := properties["unknown"].(map[string]any)
	role := properties["role"].(map[string]any)
	if bad["example"] != nil || role["example"] != nil || good["example"] != "user@example.com" {
		t.Fatalf("examples=%#v", properties)
	}
	invalid, unknown := 0, 0
	for _, d := range c.diagnostics {
		if d.Code == "FIELD_EXAMPLE_INVALID" {
			invalid++
		}
		if d.Code == "FIELD_EXAMPLE_VALIDATION_UNRESOLVED" {
			unknown++
		}
		if strings.Contains(d.Message, "user@example.com") || strings.Contains(d.Message, "not-an-email") {
			t.Fatal("diagnostic includes example value")
		}
	}
	if invalid != 2 || unknown != 1 {
		t.Fatalf("diagnostics=%#v", c.diagnostics)
	}
}

type customTimeLike struct{}

func (customTimeLike) MarshalJSON() ([]byte, error) {
	panic("custom codec must not run during description")
}

func TestTimeFieldExamplesInSchema(t *testing.T) {
	type payload struct {
		Created time.Time      `json:"created" example:"2024-01-02T03:04:05Z"`
		Updated *time.Time     `json:"updated" example:"2024-02-29T03:04:05.123456789+09:00"`
		Invalid time.Time      `json:"invalid" example:"2024-02-30T03:04:05Z"`
		Custom  customTimeLike `json:"custom" example:"2024-01-02T03:04:05Z"`
	}
	for _, mode := range []string{"request/json", "response/json"} {
		t.Run(mode, func(t *testing.T) {
			c := newCatalog(nil)
			schema := c.structure(reflect.TypeFor[payload](), mode).(map[string]any)
			properties := schema["properties"].(map[string]any)
			for field, want := range map[string]string{"created": "2024-01-02T03:04:05Z", "updated": "2024-02-29T03:04:05.123456789+09:00"} {
				value := properties[field].(map[string]any)
				if value["example"] != want || !reflect.DeepEqual(value["examples"], []any{want}) {
					t.Fatalf("%s example: %#v", field, value)
				}
				data, err := json.Marshal(value)
				if err != nil || !strings.Contains(string(data), `"format":"date-time"`) {
					t.Fatalf("%s schema: %s %v", field, data, err)
				}
			}
			if properties["invalid"].(map[string]any)["example"] != nil || properties["custom"].(map[string]any)["example"] != nil {
				t.Fatalf("invalid or custom example accepted: %#v", properties)
			}
			invalid := map[string]bool{}
			for _, d := range c.diagnostics {
				if d.Code == "FIELD_EXAMPLE_INVALID" {
					invalid[d.Target] = true
				}
			}
			if len(invalid) != 2 || !invalid["endpoint.payload.invalid"] || !invalid["endpoint.payload.custom"] {
				t.Fatalf("diagnostics: %#v", c.diagnostics)
			}
		})
	}
}
