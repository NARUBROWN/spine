package spine

import (
	"errors"
	"testing"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

type multipleResponseController struct{ handler any }

func (c *multipleResponseController) RegisterRoutes(r *api.Registry) {
	r.GET("/response", c.handler)
}

type multipleResponseFactory struct{}

func (multipleResponseFactory) Capabilities() httpcontract.Capabilities {
	return httpcontract.Capabilities{ContractVersion: httpcontract.ContractVersion}
}

func (multipleResponseFactory) New(httpcontract.Config) (httpcontract.HTTPAdapter, error) {
	panic("다중 응답을 선언한 앱의 어댑터를 생성하면 안 됩니다")
}

func TestMultipleResponsesFailBeforeAdapterCreation(t *testing.T) {
	for _, handler := range []any{
		func() (httpx.Response[string], httpx.Response[int]) {
			return httpx.Response[string]{}, httpx.Response[int]{}
		},
		func() (*httpx.Binary, *httpx.Redirect, error) { return nil, nil, nil },
	} {
		for _, openAPI := range []bool{false, true} {
			app := New(WithHTTPAdapter(multipleResponseFactory{}))
			app.Constructor(func() *multipleResponseController { return &multipleResponseController{handler: handler} })
			prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: openAPI}}})
			var failure *boot.ConfigError
			if prepared != nil || !errors.As(err, &failure) || len(failure.Issues) != 1 ||
				failure.Issues[0].Code != "HTTP_RESPONSE_RETURNS_DUPLICATED" || failure.Issues[0].Path != "Routes[0].Returns[1]" {
				t.Fatalf("다중 응답을 준비 단계에서 거부해야 합니다: %v", err)
			}
		}
	}
}
