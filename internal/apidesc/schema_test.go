package apidesc

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	validation "github.com/NARUBROWN/spine/contract/validator"
)

type sharedA struct {
	Clash string
	A     int
}
type sharedB struct {
	Clash string
	B     bool
}
type nestedDTO struct {
	Next *nestedDTO `json:"next,omitempty"`
}
type marshalDTO struct{}

type recursiveList []recursiveList
type recursiveMap map[string]recursiveMap
type pointerCycle *pointerCycle

func TestRecursiveCollectionsAndPointerCycleDoNotRecurseDuringDescription(t *testing.T) {
	c := newCatalog(nil)
	for _, typ := range []reflect.Type{reflect.TypeFor[recursiveList](), reflect.TypeFor[recursiveMap](), reflect.TypeFor[pointerCycle]()} {
		c.schema(typ, "response/json")
	}
	if len(c.schemas) != 2 || len(c.diagnostics) != 1 || c.diagnostics[0].Code != "RECURSIVE_POINTER_UNRESOLVED" {
		t.Fatalf("재귀 컨테이너 설명: %#v %+v", c.schemas, c.diagnostics)
	}
}

func (marshalDTO) MarshalJSON() ([]byte, error) {
	panic("직렬화 코드를 실행하면 안 됩니다")
}

func TestSchemaMatchesJSONNamesAndHandlesRecursiveAndSpecialTypes(t *testing.T) {
	type payload struct {
		sharedA
		sharedB
		ID       int64      `json:"id,string"`
		Hidden   string     `json:"-"`
		Optional *string    `json:"optional,omitempty"`
		Clock    time.Time  `json:"clock"`
		Bytes    []byte     `json:"bytes"`
		Node     nestedDTO  `json:"node"`
		Custom   marshalDTO `json:"custom"`
	}
	c := newCatalog(nil)
	c.schema(reflect.TypeFor[payload](), "response/json")
	id := c.ids[schemaKey{reflect.TypeFor[payload](), "response/json"}]
	schema := c.schemas[id].(map[string]any)
	properties := schema["properties"].(map[string]any)
	if properties["Clash"] != nil || properties["Hidden"] != nil || properties["A"] == nil || properties["B"] == nil || properties["id"].(map[string]any)["type"] != "string" {
		t.Fatalf("JSON 필드 해석: %#v", properties)
	}
	if properties["clock"].(map[string]any)["format"] != "date-time" {
		t.Fatalf("time.Time: %#v", properties)
	}
	if len(c.diagnostics) != 1 || c.diagnostics[0].Code != "CUSTOM_SERIALIZATION_UNRESOLVED" {
		t.Fatalf("사용자 직렬화 진단: %+v", c.diagnostics)
	}
	if _, err := json.Marshal(c.schemas); err != nil {
		t.Fatal(err)
	}
	required := schema["required"].([]string)
	for _, name := range required {
		if name == "optional" {
			t.Fatal("omitempty 필드는 응답 필수가 아닙니다")
		}
	}
}

func TestFormNamesAndRequestRequiredDoNotUseJSONOmitEmpty(t *testing.T) {
	type form struct {
		Name     string `json:"json_name" form:"display_name"`
		Internal string `json:"internal"`
	}
	c := newCatalog(nil)
	c.schema(reflect.TypeFor[form](), "request/form")
	id := c.ids[schemaKey{reflect.TypeFor[form](), "request/form"}]
	schema := c.schemas[id].(map[string]any)
	properties := schema["properties"].(map[string]any)
	if len(properties) != 1 || properties["display_name"] == nil || schema["required"] != nil {
		t.Fatalf("폼 바인딩: %#v", schema)
	}
}

func TestJSONNumberAndLiteralDashField(t *testing.T) {
	type payload struct {
		Number json.Number
		Dash   string `json:"-,omitempty"`
	}
	c := newCatalog(nil)
	c.schema(reflect.TypeFor[payload](), "response/json")
	id := c.ids[schemaKey{reflect.TypeFor[payload](), "response/json"}]
	properties := c.schemas[id].(map[string]any)["properties"].(map[string]any)
	if properties["Number"].(map[string]any)["type"] != "number" || properties["-"] == nil {
		t.Fatalf("JSON 특수 타입과 이름: %#v", properties)
	}
}

type exactIntegerValidator struct{ validation.Validator }

func (exactIntegerValidator) DescribeSchema(reflect.Type) (validation.SchemaDescription, error) {
	return validation.SchemaDescription{Fields: []validation.FieldSchema{{Index: []int{0}, Schema: json.RawMessage(`{"minimum":9223372036854775806}`)}}}, nil
}

func TestValidationConstraintsPreserveIntegerPrecision(t *testing.T) {
	type payload struct{ Value int64 }
	c := newCatalog(exactIntegerValidator{})
	c.schema(reflect.TypeFor[payload](), "request/json")
	encoded := raw(c.schemas)
	if !strings.Contains(string(encoded), `"minimum":9223372036854775806`) {
		t.Fatalf("검증 제약의 정수 정밀도를 보존해야 합니다: %s", encoded)
	}
}
