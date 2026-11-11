package httpplatform_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

type definedResponseText string
type aliasedResponseText = string
type responseRecord struct {
	Name string `json:"name"`
}
type stringContractController struct {
	handler func(*stringContractController) any
}

func (c *stringContractController) RegisterRoutes(registry *api.Registry) {
	if c.handler != nil {
		registry.GET("/response", c.handler(c))
	}
}
func (*stringContractController) Defined() httpx.Response[definedResponseText] {
	return httpx.Response[definedResponseText]{Body: "defined"}
}
func (*stringContractController) DefinedPointer() *httpx.Response[definedResponseText] {
	return &httpx.Response[definedResponseText]{Body: "defined"}
}
func (*stringContractController) Plain() httpx.Response[string] {
	return httpx.Response[string]{Body: "plain"}
}
func (*stringContractController) Alias() *httpx.Response[aliasedResponseText] {
	return &httpx.Response[aliasedResponseText]{Body: "alias"}
}
func (*stringContractController) Record() httpx.Response[responseRecord] {
	return httpx.Response[responseRecord]{Body: responseRecord{Name: "record"}}
}

func TestStringResponseContractRejectsDefinedStringsBeforeServing(t *testing.T) {
	for _, platform := range []string{"nethttp", "echo"} {
		for _, registration := range []string{"registry", "legacy"} {
			for _, test := range []struct {
				name        string
				method      any
				bound       func(*stringContractController) any
				wantBody    string
				unsupported bool
			}{
				{"defined", (*stringContractController).Defined, func(c *stringContractController) any { return c.Defined }, "", true},
				{"defined-pointer", (*stringContractController).DefinedPointer, func(c *stringContractController) any { return c.DefinedPointer }, "", true},
				{"plain", (*stringContractController).Plain, func(c *stringContractController) any { return c.Plain }, "plain", false},
				{"alias", (*stringContractController).Alias, func(c *stringContractController) any { return c.Alias }, "alias", false},
				{"record", (*stringContractController).Record, func(c *stringContractController) any { return c.Record }, `{"name":"record"}`, false},
			} {
				t.Run(platform+"/"+registration+"/"+test.name, func(t *testing.T) {
					app := spine.New(spine.WithHTTPAdapter(platformFactory(platform, nativeHooks{})))
					if registration == "registry" {
						app.Constructor(func() *stringContractController { return &stringContractController{handler: test.bound} })
					} else {
						app.Constructor(constructor.Custom(func() *stringContractController { return &stringContractController{} }))
						app.Route("GET", "/response", test.method)
					}
					options := boot.Options{HTTP: &boot.HTTPOptions{}}
					if test.unsupported {
						assertUnsupported := func(err error) {
							t.Helper()
							var configError *boot.ConfigError
							if !errors.As(err, &configError) || len(configError.Issues) == 0 || configError.Issues[0].Code != "HTTP_RETURN_UNSUPPORTED" {
								t.Fatalf("정의 문자열 응답을 사전에 거부해야 합니다: %v", err)
							}
						}
						if registration == "legacy" {
							assertUnsupported(app.Validate(options))
							_, err := spine.DescribeHTTP(app, options)
							assertUnsupported(err)
						}
						prepared, err := app.Prepare(t.Context(), options)
						if prepared != nil {
							prepared.Close(t.Context())
							t.Fatal("지원하지 않는 반환 타입으로 앱을 준비하면 안 됩니다")
						}
						assertUnsupported(err)
						return
					}
					prepared := prepare(t, app, *options.HTTP)
					address, client := managedServer(t, prepared)
					response, err := client.Get(address + "/response")
					if err != nil {
						t.Fatal(err)
					}
					defer response.Body.Close()
					body, err := io.ReadAll(response.Body)
					if test.name == "record" {
						body = []byte(strings.TrimSpace(string(body)))
					}
					if err != nil || response.StatusCode != http.StatusOK || string(body) != test.wantBody {
						t.Fatalf("지원하는 응답을 보존해야 합니다: %d %q %v", response.StatusCode, body, err)
					}
				})
			}
		}
	}
}
