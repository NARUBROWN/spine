package spine

import (
	"errors"
	"github.com/NARUBROWN/spine/pkg/boot"
	"testing"
)

type explicitPlainBody struct {
	Name string `json:"name"`
}
type explicitPlainForm struct {
	Name string `form:"name"`
}
type explicitPlainQuery struct {
	Name string `query:"name"`
}
type explicitPlainHeader struct {
	Name string `header:"X-Name"`
}
type explicitPlainPath struct {
	ID string `path:"id"`
}
type explicitPlainPointer = *explicitPlainBody
type explicitPlainController struct{}

func (*explicitPlainController) Body(*explicitPlainBody)     {}
func (*explicitPlainController) Form(*explicitPlainForm)     {}
func (*explicitPlainController) Query(*explicitPlainQuery)   {}
func (*explicitPlainController) Header(*explicitPlainHeader) {}
func (*explicitPlainController) Path(*explicitPlainPath)     {}
func (*explicitPlainController) Alias(explicitPlainPointer)  {}
func TestUnwrappedHTTPDTOsFailBeforeAdapterAndConstruction(t *testing.T) {
	for _, handler := range []any{(*explicitPlainController).Body, (*explicitPlainController).Form, (*explicitPlainController).Query, (*explicitPlainController).Header, (*explicitPlainController).Path, (*explicitPlainController).Alias} {
		app := New(WithHTTPAdapter(&repeatedInputFactory{}))
		app.Constructor(func() *explicitPlainController { panic("must reject before construction") })
		app.Route("POST", "/users/:id", handler)
		options := boot.Options{HTTP: &boot.HTTPOptions{}}
		assert := func(err error) {
			t.Helper()
			var configuration *boot.ConfigError
			if !errors.As(err, &configuration) || len(configuration.Issues) != 1 || configuration.Issues[0].Code != "HTTP_INPUT_WRAPPER_REQUIRED" {
				t.Fatalf("missing explicit wrapper diagnostic: %v", err)
			}
		}
		assert(app.Validate(options))
		prepared, err := app.Prepare(t.Context(), options)
		if prepared != nil {
			t.Fatal("unexpected prepared application")
		}
		assert(err)
	}
}
