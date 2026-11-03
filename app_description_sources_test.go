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
	"github.com/NARUBROWN/spine/pkg/query"
	"github.com/NARUBROWN/spine/pkg/route"
)

type searchDTO struct {
	Count int    `query:"count"`
	Bytes []byte `query:"bytes"`
}
type headersDTO struct {
	ID string `header:"X-Request-ID"`
}
type uploadDTO struct {
	File  *multipart.FileHeader   `form:"avatar"`
	Files []*multipart.FileHeader `form:"attachments"`
}

func (*descriptionController) Search(query.DTO[searchDTO], *header.DTO[headersDTO]) httpx.Response[descriptionDTO] {
	panic("설명 중 실행하면 안 됩니다")
}
func (*descriptionController) Upload(*uploadDTO) httpx.Binary {
	panic("설명 중 실행하면 안 됩니다")
}

func TestDescribeSourceDTOFilesAndDeclaredResponses(t *testing.T) {
	app := New()
	app.Route("GET", "/search", (*descriptionController).Search, route.Response[descriptionDTO](201), route.ErrorResponse(409))
	app.Route("POST", "/upload", (*descriptionController).Upload, route.ResponseAs[[]byte](200, "image/png"))
	desc, err := DescribeHTTP(app, boot.Options{HTTP: &boot.HTTPOptions{}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := openapi.Generate(desc, boot.OpenAPIOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{`"in":"header"`, `"name":"count"`, `"name":"X-Request-Id"`, `"201":`, `"409":`, `"image/png"`, `"format":"binary"`} {
		if !strings.Contains(string(data), token) {
			t.Fatalf("입력과 응답 계약 누락 %s: %s", token, data)
		}
	}
	search := document["paths"].(map[string]any)["/search"].(map[string]any)["get"].(map[string]any)
	if search["requestBody"] != nil {
		t.Fatalf("쿼리와 헤더를 본문으로 설명하면 안 됩니다: %s", data)
	}
	for _, parameter := range search["parameters"].([]any) {
		param := parameter.(map[string]any)
		if param["name"] == "bytes" && strings.Contains(string(rawJSON(param["schema"])), "base64") {
			t.Fatal("쿼리의 []byte는 숫자 목록입니다")
		}
	}
}

func rawJSON(value any) []byte { data, _ := json.Marshal(value); return data }

func TestInvalidDeclaredResponsesAreRejected(t *testing.T) {
	for _, option := range []func(App){
		func(app App) { app.Route("GET", "/", (*descriptionController).Get, route.Response[string](99)) },
		func(app App) {
			app.Route("GET", "/", (*descriptionController).Get, route.ResponseAs[string](200, "text/plain; charset=utf-8"))
		},
		func(app App) { app.Route("GET", "/", (*descriptionController).Get, route.ErrorResponse(200)) },
	} {
		app := New()
		option(app)
		if _, err := DescribeHTTP(app, boot.Options{HTTP: &boot.HTTPOptions{}}); err == nil {
			t.Fatal("잘못된 응답 선언을 거부해야 합니다")
		}
	}
}
