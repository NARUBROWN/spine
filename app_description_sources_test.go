package spine

import (
	"encoding/json"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/header"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/openapi"
)

type searchDTO struct {
	Count int `query:"count"`
}
type uploadDTO struct {
	File  *multipart.FileHeader   `form:"avatar"`
	Files []*multipart.FileHeader `form:"attachments"`
}

func (*descriptionController) Search(*searchDTO, header.Values) httpx.Response[descriptionDTO] {
	panic("설명 중 실행하면 안 됩니다")
}
func (*descriptionController) Upload(*uploadDTO) httpx.Binary {
	panic("설명 중 실행하면 안 됩니다")
}

func TestDescribeExistingInputsAndReturnTypesWithoutAdditionalDeclarations(t *testing.T) {
	app := New()
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
	for _, token := range []string{`"format":"binary"`, `"multipart/form-data"`, `"DTO_QUERY_BINDING_UNRESOLVED"`, `"DYNAMIC_PARAMETERS_UNRESOLVED"`, `"RESPONSE_STATUS_DYNAMIC"`} {
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
