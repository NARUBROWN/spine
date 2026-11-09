package openapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	playground "github.com/NARUBROWN/spine-validator-playground"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
)

type scalarPathController struct{}

func (*scalarPathController) Constructor() *scalarPathController { return &scalarPathController{} }
func (*scalarPathController) RegisterRoutes(r *api.Registry) {
	r.GET("/users/:id", func(id input.Path[int8]) httpx.Response[int8] { return httpx.Response[int8]{Body: *id} })
	r.GET("/users/:id/:name", func(ctx context.Context, id input.Path[int64], name input.Path[string]) httpx.Response[string] {
		return httpx.Response[string]{Body: *name}
	})
}
func scalarApp(t *testing.T) spine.App {
	t.Helper()
	v, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	app := spine.New(platform.Option(), spine.WithValidator(v))
	app.Constructor(constructor.Controller(&scalarPathController{}))
	return app
}
func TestScalarPathHTTPAndOpenAPI(t *testing.T) {
	app := scalarApp(t)
	p, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close(context.Background())
	server := httptest.NewServer(mustHTTPHandler(t, p))
	defer server.Close()
	for _, tc := range []struct {
		path   string
		status int
		body   string
	}{{"/users/127", 200, "127"}, {"/users/-128", 200, "-128"}, {"/users/128", 400, ""}, {"/users/nope", 400, ""}} {
		resp, err := http.Get(server.URL + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tc.status || tc.body != "" && strings.TrimSpace(string(data)) != tc.body {
			t.Fatalf("%s: %d %s", tc.path, resp.StatusCode, data)
		}
	}
	resp, err := http.Get(server.URL + "/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	params := operation(doc, "/users/{id}", "get")["parameters"].([]any)
	if len(params) != 1 {
		t.Fatal(params)
	}
	param := params[0].(map[string]any)
	schema := param["schema"].(map[string]any)
	if param["name"] != "id" || param["in"] != "path" || param["required"] != true || schema["type"] != "integer" || schema["minimum"] != float64(-128) || schema["maximum"] != float64(127) {
		t.Fatalf("%v", param)
	}
	params = operation(doc, "/users/{id}/{name}", "get")["parameters"].([]any)
	if len(params) != 2 {
		t.Fatalf("expected two path parameters: %v", params)
	}
	for index, want := range []struct{ name, typ string }{{"id", "integer"}, {"name", "string"}} {
		param := params[index].(map[string]any)
		if param["name"] != want.name || param["in"] != "path" || param["required"] != true || param["schema"].(map[string]any)["type"] != want.typ {
			t.Fatalf("parameter %d: %v", index, param)
		}
	}
}
