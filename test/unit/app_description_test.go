package spine

import httpinput "github.com/NARUBROWN/spine/pkg/input"

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/contract/description"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/openapi"
)

type descriptionDTO struct {
	Name string `json:"name" validate:"required"`
}
type descriptionPath struct {
	ID int64 `path:"id"`
}
type descriptionController struct{}

func (*descriptionController) Handle() {}

func TestDescribeRegistrationsCopiesPublicMetadataWithoutInitializing(t *testing.T) {
	app := New()
	app.Constructor(func() *descriptionController { panic("생성자를 실행하면 안 됩니다") })
	handler := (*descriptionController).Handle
	if err := app.Consumers().Register("orders", handler); err != nil {
		t.Fatal(err)
	}
	if err := app.WebSocket().Register("/chat", handler); err != nil {
		t.Fatal(err)
	}
	for _, registrar := range []Registrar{app.Consumers(), app.WebSocket()} {
		registry := reflect.TypeOf(registrar)
		for _, method := range []string{"Registrations", "Snapshot"} {
			if _, ok := registry.MethodByName(method); ok {
				t.Fatalf("등록 경로에 내부 메서드가 노출되었습니다: %s", method)
			}
		}
	}
	receiver := reflect.TypeFor[descriptionController]()
	wantHandler := description.Handler{PackagePath: receiver.PkgPath(), Receiver: receiver.Name(), Method: "Handle"}
	want := description.Registrations{
		Consumers:  []description.ConsumerRegistration{{Topic: "orders", Handler: wantHandler}},
		WebSockets: []description.WebSocketRegistration{{Path: "/chat", Handler: wantHandler}},
	}
	got, err := DescribeRegistrations(app)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("등록 설명이 다릅니다: %+v, %v", got, err)
	}
	got.Consumers[0].Handler.Method = "changed"
	got.WebSockets[0].Path = "/changed"
	again, err := DescribeRegistrations(app)
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Fatalf("설명 결과의 수정이 앱 등록에 반영되었습니다: %+v, %v", again, err)
	}
}

func (*descriptionController) Create(httpinput.Body[descriptionDTO]) httpx.Response[descriptionDTO] {
	panic("핸들러를 실행하면 안 됩니다")
}
func (*descriptionController) Get(httpinput.Path[descriptionPath]) httpx.Response[*descriptionDTO] {
	panic("핸들러를 실행하면 안 됩니다")
}

type descriptionValidator struct{}

func (*descriptionValidator) Validate(context.Context, any) error {
	panic("검증을 실행하면 안 됩니다")
}
func (*descriptionValidator) DescribeSchema(t reflect.Type) (validation.SchemaDescription, error) {
	return validation.SchemaDescription{Fields: []validation.FieldSchema{{Index: []int{0}, Required: true, Schema: json.RawMessage(`{"minLength":1}`)}}}, nil
}

func TestDescribeHTTPDoesNotInitializeApplication(t *testing.T) {
	app := New(WithValidator(&descriptionValidator{}))
	app.Constructor(func() *descriptionController { panic("생성자를 실행하면 안 됩니다") })
	app.Route("POST", "/users", (*descriptionController).Create)
	for _, p := range []string{"/users/:id", "/members/:id"} {
		app.Route("GET", p, (*descriptionController).Get)
	}
	options := boot.Options{HTTP: &boot.HTTPOptions{GlobalPrefix: "/api"}}
	desc, err := DescribeHTTP(app, options)
	if err != nil || len(desc.Routes) != 3 {
		t.Fatalf("설명 결과: %+v %v", desc, err)
	}
	document, err := openapi.Generate(desc, boot.OpenAPIOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		OpenAPI string `json:"openapi"`
		Paths   map[string]map[string]struct {
			OperationID string `json:"operationId"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(document, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.OpenAPI != "3.1.2" || len(doc.Paths) != 3 || doc.Paths["/api/users/{id}"]["get"].OperationID == doc.Paths["/api/members/{id}"]["get"].OperationID {
		t.Fatalf("OpenAPI 경로: %s", document)
	}
	if !strings.Contains(string(document), `"minLength":1`) {
		t.Fatalf("등록 검증기의 제약이 누락됐습니다: %s", document)
	}
	desc.Routes[0].Path = "/mutated"
	for key := range desc.Schemas {
		desc.Schemas[key] = json.RawMessage(`false`)
	}
	again, err := DescribeHTTP(app, options)
	if err != nil || again.Routes[0].Path == "/mutated" {
		t.Fatalf("반환값 수정이 앱에 반영됐습니다: %v", err)
	}
}

func TestDescribeWithoutValidatorDoesNotInterpretValidationTags(t *testing.T) {
	app := New()
	app.Route("POST", "/users", (*descriptionController).Create)
	desc, err := DescribeHTTP(app, boot.Options{HTTP: &boot.HTTPOptions{}})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := openapi.Generate(desc, boot.OpenAPIOptions{})
	if err != nil || strings.Contains(string(doc), `minLength`) {
		t.Fatalf("미등록 검증 규칙을 추론했습니다: %s %v", doc, err)
	}
	for _, route := range desc.Routes {
		for _, input := range route.Inputs {
			if input.Validation {
				t.Fatal("미등록 검증기가 활성화됐습니다")
			}
		}
	}
}

func TestDescribeRejectsRepeatedRoutePattern(t *testing.T) {
	app := New()
	app.Route("GET", "/users/:id", (*descriptionController).Get)
	app.Route("GET", "/users/:key", (*descriptionController).Get)
	if _, err := DescribeHTTP(app, boot.Options{HTTP: &boot.HTTPOptions{}}); err == nil {
		t.Fatal("같은 메서드와 경로 형태의 중복 등록을 거부해야 합니다")
	}
}
