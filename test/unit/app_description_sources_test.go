package spine

import (
	"context"
	"encoding/json"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"mime/multipart"
	"reflect"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/openapi"
)

type searchDTO struct {
	Count int `query:"count"`
}
type headersDTO struct {
	ID string `header:"X-Request-ID"`
}

type uploadDTO struct {
	File  *multipart.FileHeader   `form:"avatar"`
	Files []*multipart.FileHeader `form:"attachments"`
}

func (*descriptionController) Search(*searchDTO, *headersDTO) httpx.Response[descriptionDTO] {
	panic("설명 중 실행하면 안 됩니다")
}
func (*descriptionController) Upload(*uploadDTO) httpx.Binary {
	panic("설명 중 실행하면 안 됩니다")
}

func TestDescribeExistingInputsAndReturnTypesWithoutAdditionalDeclarations(t *testing.T) {
	app := New(WithValidator(&schemaOnlyValidator{}))
	app.Route("GET", "/search", (*descriptionController).Search)
	app.Route("POST", "/upload", (*descriptionController).Upload)
	desc, err := DescribeHTTP(app, boot.Options{HTTP: &boot.HTTPOptions{}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := openapi.Generate(desc, boot.OpenAPIOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{`"format":"binary"`, `"multipart/form-data"`, `"in":"query"`, `"in":"header"`, `"RESPONSE_STATUS_DYNAMIC"`} {
		if !strings.Contains(string(data), token) {
			t.Fatalf("기존 입력과 미해결 진단 누락 %s: %s", token, data)
		}
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	search := document["paths"].(map[string]any)["/search"].(map[string]any)["get"].(map[string]any)
	responses := search["responses"].(map[string]any)
	if responses["default"] == nil || responses["200"] != nil {
		t.Fatalf("반환형에서 상태 코드를 추측하면 안 됩니다: %s", data)
	}
}

type schemaOnlyValidator struct{}

func (*schemaOnlyValidator) Validate(context.Context, any) error {
	panic("설명 중 검증하면 안 됩니다")
}
func (*schemaOnlyValidator) DescribeSchema(reflect.Type) (validation.SchemaDescription, error) {
	return validation.SchemaDescription{}, nil
}

func TestInputSchemasRequireAConfiguredSchemaDescriber(t *testing.T) {
	app := New()
	app.Route("POST", "/users", (*descriptionController).Create)
	app.Route("GET", "/search", (*descriptionController).Search)
	desc, err := DescribeHTTP(app, boot.Options{HTTP: &boot.HTTPOptions{}})
	if err != nil {
		t.Fatal(err)
	}
	document, err := openapi.Generate(desc, boot.OpenAPIOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(document, &doc); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/users", "/search"} {
		for _, raw := range doc["paths"].(map[string]any)[path].(map[string]any) {
			operation := raw.(map[string]any)
			if operation["requestBody"] != nil || operation["parameters"] != nil {
				t.Fatalf("검증 설명 계약 없이 상세 입력을 표시하면 안 됩니다: %s", document)
			}
		}
	}
	if !desc.PathsOnly || !strings.Contains(string(document), "VALIDATOR_NOT_CONFIGURED") || doc["components"] != nil {
		t.Fatal("검증기 미등록 시 경로만 설명하고 생략 이유를 남겨야 합니다")
	}
	for _, item := range doc["paths"].(map[string]any) {
		for _, raw := range item.(map[string]any) {
			responses := raw.(map[string]any)["responses"].(map[string]any)
			if len(responses) != 1 || responses["default"].(map[string]any)["content"] != nil {
				t.Fatalf("응답 스키마나 상태 코드를 추정하면 안 됩니다: %s", document)
			}
		}
	}
}

type mixedInputDTO struct {
	Q  string `query:"q"`
	ID string `header:"X-ID"`
}

func (*descriptionController) Mixed(*mixedInputDTO) error { return nil }
func TestMixedInputSourcesAreRejectedEvenWithOpenAPIDisabled(t *testing.T) {
	app := New()
	app.Route("GET", "/mixed", (*descriptionController).Mixed)
	err := app.Validate(boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{}}})
	if err == nil || !strings.Contains(err.Error(), "HTTP_INPUT_CONTRACT_INVALID") {
		t.Fatalf("출처를 분리해야 합니다: %v", err)
	}
}

type validationWithoutDescription struct{}

func (*validationWithoutDescription) Validate(context.Context, any) error {
	panic("설명 중 검증하면 안 됩니다")
}
func TestValidatorWithoutDescriptionDoesNotExposeInputSchemas(t *testing.T) {
	app := New(WithValidator(&validationWithoutDescription{}))
	app.Route("GET", "/search", (*descriptionController).Search)
	desc, err := DescribeHTTP(app, boot.Options{HTTP: &boot.HTTPOptions{}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := openapi.Generate(desc, boot.OpenAPIOptions{})
	if err != nil || !strings.Contains(string(data), "INPUT_SCHEMA_DESCRIPTION_UNAVAILABLE") || strings.Contains(string(data), `"parameters"`) {
		t.Fatalf("선택한 검증기의 설명 기능이 필요합니다: %s %v", data, err)
	}
}
