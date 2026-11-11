package spine

import (
	"errors"
	"testing"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/input"
)

type conflictingXMLBody struct {
	First  string `json:"first" xml:"same"`
	Second string `json:"second" xml:"same>child"`
}

type conflictingXMLController struct{}

func (*conflictingXMLController) RegisterRoutes(r *api.Registry) {
	r.POST("/xml", func(input.Body[conflictingXMLBody]) { panic("핸들러를 실행하면 안 됩니다") })
}

type conflictingXMLFactory struct{}

func (conflictingXMLFactory) Capabilities() httpcontract.Capabilities {
	return httpcontract.Capabilities{ContractVersion: httpcontract.ContractVersion}
}

func (conflictingXMLFactory) New(httpcontract.Config) (httpcontract.HTTPAdapter, error) {
	panic("XML 충돌이 있는 앱의 어댑터를 생성하면 안 됩니다")
}

func TestControllerXMLConflictsFailBeforeAdapterCreation(t *testing.T) {
	for _, openAPI := range []bool{false, true} {
		for _, validator := range []bool{false, true} {
			options := []Option{WithHTTPAdapter(conflictingXMLFactory{})}
			if validator {
				options = append(options, WithValidator(&repeatedInputValidator{}))
			}
			app := New(options...)
			app.Constructor(func() *conflictingXMLController { return &conflictingXMLController{} })
			prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: openAPI}}})
			var configError *boot.ConfigError
			if prepared != nil || !errors.As(err, &configError) || len(configError.Issues) != 1 ||
				configError.Issues[0].Code != "HTTP_INPUT_CONTRACT_INVALID" || configError.Issues[0].Path != "Routes[0].Parameters[0]" {
				t.Fatalf("XML 충돌을 준비 단계에서 거부해야 합니다: openAPI=%v validator=%v error=%v", openAPI, validator, err)
			}
		}
	}
}
