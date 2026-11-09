package httpplatform_test

import (
	"io"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
)

type scalarPathController struct{}

func (*scalarPathController) Constructor() *scalarPathController { return &scalarPathController{} }
func (*scalarPathController) RegisterRoutes(r *api.Registry) {
	r.GET("/scalar/:id", func(id input.Path[int16]) httpx.Response[int16] { return httpx.Response[int16]{Body: *id} })
}
func TestScalarPathAcrossHTTPPlatforms(t *testing.T) {
	for _, name := range []string{"echo", "nethttp"} {
		t.Run(name, func(t *testing.T) {
			app := spine.New(spine.WithHTTPAdapter(platformFactory(name, nativeHooks{})))
			app.Constructor(constructor.Controller(&scalarPathController{}))
			p := prepare(t, app, boot.HTTPOptions{})
			base, client := managedServer(t, p)
			for _, tc := range []struct {
				path   string
				status int
				body   string
			}{{"/scalar/32767", 200, "32767"}, {"/scalar/32768", 400, ""}, {"/scalar/invalid", 400, ""}} {
				resp, err := client.Get(base + tc.path)
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if resp.StatusCode != tc.status || tc.body != "" && strings.TrimSpace(string(data)) != tc.body {
					t.Fatalf("%s: %d %s", tc.path, resp.StatusCode, data)
				}
			}
		})
	}
}
