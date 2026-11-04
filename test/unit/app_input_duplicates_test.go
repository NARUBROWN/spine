package spine

import (
	"context"
	"errors"
	"reflect"
	"testing"

	adapter "github.com/NARUBROWN/spine/contract/http"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type repeatedBody struct {
	Name string `json:"name"`
}
type repeatedBodyAlias = repeatedBody
type repeatedQuery struct {
	Name string `query:"name"`
}
type repeatedHeader struct {
	Name string `header:"X-Name"`
}
type repeatedInputController struct{}

func (*repeatedInputController) Body(*repeatedBody, *repeatedBody) error {
	panic("핸들러를 실행하면 안 됩니다")
}
func (*repeatedInputController) Query(*repeatedQuery, *repeatedQuery) error {
	panic("핸들러를 실행하면 안 됩니다")
}
func (*repeatedInputController) Header(*repeatedHeader, *repeatedHeader) error {
	panic("핸들러를 실행하면 안 됩니다")
}
func (*repeatedInputController) Alias(*repeatedBody, *repeatedBodyAlias) error {
	panic("핸들러를 실행하면 안 됩니다")
}
func (*repeatedInputController) Single(*repeatedBody) error {
	panic("핸들러를 실행하면 안 됩니다")
}

type repeatedInputFactory struct{}

func (*repeatedInputFactory) Capabilities() adapter.Capabilities {
	panic("어댑터 기능 조회보다 먼저 실패해야 합니다")
}
func (*repeatedInputFactory) New(adapter.Config) (adapter.HTTPAdapter, error) {
	panic("어댑터를 생성하면 안 됩니다")
}

type repeatedInputValidator struct{}

func (*repeatedInputValidator) Validate(context.Context, any) error {
	panic("검증기를 실행하면 안 됩니다")
}
func (*repeatedInputValidator) DescribeSchema(reflect.Type) (validation.SchemaDescription, error) {
	panic("중복 입력의 스키마를 설명하면 안 됩니다")
}

func TestRepeatedInputDTOsFailBeforeApplicationInitialization(t *testing.T) {
	for _, handler := range []any{
		(*repeatedInputController).Body, (*repeatedInputController).Query,
		(*repeatedInputController).Header, (*repeatedInputController).Alias,
	} {
		for _, enabled := range []bool{false, true} {
			app := New(WithHTTPAdapter(&repeatedInputFactory{}), WithValidator(&repeatedInputValidator{}))
			app.Constructor(func() *repeatedInputController { panic("생성자를 실행하면 안 됩니다") })
			app.Route("POST", "/repeat", handler)
			options := boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: enabled}}}
			assertDuplicateInput := func(err error) {
				t.Helper()
				var failure *boot.ConfigError
				if !errors.As(err, &failure) || len(failure.Issues) != 1 || failure.Issues[0].Code != "HTTP_INPUT_DTO_DUPLICATED" || failure.Issues[0].Path != "Routes[0].Parameters[1]" {
					t.Fatalf("중복 입력을 사전 검사에서 거부해야 합니다: %v", err)
				}
			}
			assertDuplicateInput(app.Validate(options))
			prepared, err := app.Prepare(t.Context(), options)
			if prepared != nil {
				t.Fatal("잘못된 입력으로 앱을 준비하면 안 됩니다")
			}
			assertDuplicateInput(err)
			assertDuplicateInput(app.RunContext(t.Context(), options))
			_, err = DescribeHTTP(app, options)
			assertDuplicateInput(err)
		}
	}
}

func TestSeparateRoutesMayReuseTheSameInputDTO(t *testing.T) {
	app := New()
	app.Route("POST", "/first", (*repeatedInputController).Single)
	app.Route("POST", "/second", (*repeatedInputController).Single)
	if err := app.Validate(boot.Options{}); err != nil {
		t.Fatalf("다른 라우트에서의 DTO 재사용은 허용해야 합니다: %v", err)
	}
	if _, err := DescribeHTTP(app, boot.Options{HTTP: &boot.HTTPOptions{}}); err != nil {
		t.Fatal(err)
	}
}
