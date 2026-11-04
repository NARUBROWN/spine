package apidesc

import (
	"encoding/json"
	"testing"

	"github.com/NARUBROWN/spine/contract/description"
	"github.com/NARUBROWN/spine/pkg/boot"
)

func TestOpenAPIPreservesWildcardBinaryAndEscapesLiteralTemplateCharacters(t *testing.T) {
	contract := description.HTTP{Schemas: map[string]json.RawMessage{}, Routes: []description.Route{{Method: "GET", Path: "/literal{value}", Handler: "Binary", Returns: []description.Return{{Kind: "binary", Content: map[string]json.RawMessage{"*/*": json.RawMessage(`{}`)}}}}}}
	data, err := OpenAPI(contract, boot.OpenAPIOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	paths := doc["paths"].(map[string]any)
	item := paths["/literal%7Bvalue%7D"].(map[string]any)
	content := item["get"].(map[string]any)["responses"].(map[string]any)["default"].(map[string]any)["content"].(map[string]any)
	if len(content) != 1 || content["*/*"] == nil {
		t.Fatalf("바이너리의 JSON MIME을 오류 형식으로 한정하면 안 됩니다: %s", data)
	}
}

func TestOpenAPIOmitsUnrepresentableMethodsAndTemplatesWithDiagnostics(t *testing.T) {
	contract := description.HTTP{Schemas: map[string]json.RawMessage{}, Routes: []description.Route{{Method: "CONNECT", Path: "/tunnel"}, {Method: "GET", Path: "/:"}}}
	data, err := OpenAPI(contract, boot.OpenAPIOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths       map[string]any           `json:"paths"`
		Diagnostics []description.Diagnostic `json:"x-spine-diagnostics"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Paths) != 0 || len(doc.Diagnostics) != 2 {
		t.Fatalf("표현 불가 등록: %s", data)
	}
}

func TestOpenAPIUnifiesTemplateNamesAcrossMethods(t *testing.T) {
	contract := description.HTTP{Schemas: map[string]json.RawMessage{}}
	for _, spec := range []struct{ method, name string }{{"GET", "id"}, {"POST", "userID"}} {
		contract.Routes = append(contract.Routes, description.Route{Method: spec.method, Path: "/users/:" + spec.name, Inputs: []description.Input{{Binding: "path", Name: spec.name, Required: true, Schema: json.RawMessage(`{"type":"integer"}`)}}})
	}
	data, err := OpenAPI(contract, boot.OpenAPIOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Parameters []struct{ Name string }
		}
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Paths) != 1 || len(doc.Paths["/users/{id}"]) != 2 || doc.Paths["/users/{id}"]["post"].Parameters[0].Name != "id" {
		t.Fatalf("같은 URL 형태는 메서드별로 중복 Path Item을 만들면 안 됩니다: %s", data)
	}
}
