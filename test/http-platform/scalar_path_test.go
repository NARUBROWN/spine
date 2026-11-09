package httpplatform_test

import (
	"context"
	"io"
	"net/http"
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

type scalarPathQuery struct{ Limit int }
type scalarPathBody struct{ Message string }
type scalarPathResult struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Limit   int    `json:"limit,omitempty"`
	Message string `json:"message,omitempty"`
}

func (*scalarPathController) Constructor() *scalarPathController { return &scalarPathController{} }
func (*scalarPathController) RegisterRoutes(r *api.Registry) {
	r.GET("/scalar/:id", func(id input.Path[int16]) httpx.Response[int16] { return httpx.Response[int16]{Body: *id} })
	r.GET("/scalar/:id/:name", (*scalarPathController).Multiple)
	r.GET("/strings/:first/:second", func(first input.Path[string], second input.Path[string]) httpx.Response[string] {
		return httpx.Response[string]{Body: *first + ":" + *second}
	})
	r.POST("/interleaved/:id/:name", func(ctx context.Context, id input.Path[int64], query input.Query[scalarPathQuery], name input.Path[string], body input.Body[scalarPathBody]) httpx.Response[scalarPathResult] {
		return httpx.Response[scalarPathResult]{Body: scalarPathResult{ID: *id, Name: *name, Limit: query.Limit, Message: body.Message}}
	})
}

func (*scalarPathController) Multiple(id input.Path[int64], name input.Path[string]) httpx.Response[scalarPathResult] {
	return httpx.Response[scalarPathResult]{Body: scalarPathResult{ID: *id, Name: *name}}
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

func TestMultipleScalarPathsAcrossHTTPPlatforms(t *testing.T) {
	for _, name := range []string{"echo", "nethttp"} {
		t.Run(name, func(t *testing.T) {
			app := spine.New(spine.WithHTTPAdapter(platformFactory(name, nativeHooks{})))
			app.Constructor(constructor.Controller(&scalarPathController{}))
			p := prepare(t, app, boot.HTTPOptions{})
			base, client := managedServer(t, p)
			for _, tc := range []struct {
				method, path, body string
				status             int
				want               string
			}{
				{"GET", "/scalar/42/alice", "", 200, `{"id":42,"name":"alice"}`},
				{"GET", "/scalar/9223372036854775807/%ED%95%9C%EA%B8%80", "", 200, `{"id":9223372036854775807,"name":"한글"}`},
				{"GET", "/scalar/-9223372036854775808/min", "", 200, `{"id":-9223372036854775808,"name":"min"}`},
				{"GET", "/scalar/9223372036854775808/overflow", "", 400, ""},
				{"GET", "/scalar/invalid/alice", "", 400, ""},
				{"GET", "/strings/first/second", "", 200, "first:second"},
				{"POST", "/interleaved/7/bob?limit=3", `{"Message":"hello"}`, 200, `{"id":7,"name":"bob","limit":3,"message":"hello"}`},
			} {
				req, err := http.NewRequest(tc.method, base+tc.path, strings.NewReader(tc.body))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Content-Type", "application/json")
				resp, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if resp.StatusCode != tc.status {
					t.Fatalf("%s: %d %s", tc.path, resp.StatusCode, data)
				}
				if tc.want != "" && strings.TrimSpace(string(data)) != tc.want {
					t.Fatalf("%s: got %s want %s", tc.path, data, tc.want)
				}
			}
		})
	}
}
