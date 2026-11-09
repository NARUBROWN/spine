package test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/openapi/ui/redoc"
	"github.com/NARUBROWN/spine/pkg/openapi/ui/swaggerui"
)

func TestOpenAPIUIEmbedsAssetsAndPreservesBusinessPipeline(t *testing.T) {
	app := setupApp()
	interceptor := &openAPIInterceptor{}
	app.Interceptor(interceptor)
	options := boot.OpenAPIOptions{Enabled: true, UI: []boot.OpenAPIUI{swaggerui.New(swaggerui.Options{}), redoc.New(redoc.Options{})}}
	h := newTestHandlerFromAppWithOptions(t, app, boot.Options{HTTP: &boot.HTTPOptions{GlobalPrefix: "/api", OpenAPI: options}})
	for _, resource := range []struct{ path, media, contains string }{
		{"/docs", "text/html", "swagger-ui"},
		{"/redoc", "text/html", `id="redoc"`},
		{"/docs/_assets/swagger-ui-bundle.js", "text/javascript", "SwaggerUIBundle"},
		{"/docs/_assets/swagger-ui.css", "text/css", ".swagger-ui"},
		{"/docs/_assets/init.js", "text/javascript", "validatorUrl:null"},
		{"/redoc/_assets/redoc.standalone.js", "text/javascript", "Redoc"},
		{"/redoc/_assets/init.js", "text/javascript", "disableGoogleFont:true"},
		{"/docs/_assets/LICENSE", "text/plain", "Apache License"},
		{"/docs/_assets/NOTICE", "text/plain", "SmartBear"},
		{"/docs/_assets/THIRD_PARTY_LICENSES.txt", "text/plain", "Copyright"},
		{"/redoc/_assets/LICENSE", "text/plain", "MIT License"},
		{"/redoc/_assets/THIRD_PARTY_LICENSES.txt", "text/plain", "Copyright"},
	} {
		t.Run(resource.path, func(t *testing.T) {
			for _, method := range []string{"GET", "HEAD"} {
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest(method, resource.path, nil))
				if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), resource.media) {
					t.Fatalf("Resource response: %s %s %d %v", method, resource.path, w.Code, w.Header())
				}
				if method == "HEAD" && w.Body.Len() != 0 {
					t.Fatal("HEAD response must not contain a body")
				}
				if method == "GET" && !strings.Contains(w.Body.String(), resource.contains) {
					t.Fatal("Embedded content is missing")
				}
			}
		})
	}
	if interceptor.calls.Load() != 0 {
		t.Fatal("Documentation resources executed business interceptors")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/hello", nil))
	if w.Code != 200 || interceptor.calls.Load() != 1 {
		t.Fatalf("Existing API response: %d", w.Code)
	}
}

func TestOpenAPIUIRequiresExplicitSelectionAndActivation(t *testing.T) {
	for _, options := range []boot.OpenAPIOptions{
		{},
		{Enabled: true},
		{UI: []boot.OpenAPIUI{func(string) ([]boot.OpenAPIResource, error) { panic("Disabled UI provider was invoked") }}},
		{Enabled: true, UI: []boot.OpenAPIUI{swaggerui.New(swaggerui.Options{})}},
	} {
		h := newTestHandlerFromAppWithOptions(t, setupApp(), boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: options}})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/redoc", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("Unselected ReDoc response: %d", w.Code)
		}
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/docs", nil))
		if w.Code == 200 {
			t.Fatal("Documentation UI handled a POST request")
		}
	}
}

func TestOpenAPIUIRejectsRouteAndResourceCollisions(t *testing.T) {
	for _, route := range []string{"/docs", "/docs/_assets/init.js", "/docs/:kind/:name", "/:id"} {
		app := setupApp()
		app.Route("GET", route, (*appCtrl).Hello)
		err := app.Validate(boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true, Path: "/spec/openapi.json", UI: []boot.OpenAPIUI{swaggerui.New(swaggerui.Options{})}}}})
		if err == nil || !strings.Contains(err.Error(), "OPENAPI_PATH_CONFLICT") {
			t.Fatalf("Path collision check %s: %v", route, err)
		}
	}
	for _, providers := range [][]boot.OpenAPIUI{
		{nil},
		{swaggerui.New(swaggerui.Options{Path: "/openapi.json"})},
		{swaggerui.New(swaggerui.Options{Path: "relative"})},
		{swaggerui.New(swaggerui.Options{}), redoc.New(redoc.Options{Path: "/docs"})},
		{func(string) ([]boot.OpenAPIResource, error) { return nil, errors.New("provider failed") }},
		{func(string) ([]boot.OpenAPIResource, error) {
			return []boot.OpenAPIResource{{Path: "/bad", ContentType: "text/html\r\nX-Test: value"}}, nil
		}},
	} {
		if err := setupApp().Validate(boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true, UI: providers}}}); err == nil {
			t.Fatal("Invalid UI configuration was accepted")
		}
	}
}

func TestOpenAPIUIUsesCustomDocumentAndEscapesTitle(t *testing.T) {
	options := boot.OpenAPIOptions{Enabled: true, Path: "/internal/spec.json", UI: []boot.OpenAPIUI{swaggerui.New(swaggerui.Options{Path: "/internal/docs", Title: "</title><script>bad()</script>"})}}
	h := newTestHandlerFromAppWithOptions(t, setupApp(), boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: options}})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/internal/docs", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "<script>bad()") {
		t.Fatal("Title was not escaped against HTML injection")
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/internal/docs/_assets/init.js", nil))
	if !strings.Contains(w.Body.String(), `new URL("spec.json",window.location.href)`) {
		t.Fatalf("Custom document path: %s", w.Body)
	}
}

func TestOpenAPIUIServesTheValidatedResourceSnapshot(t *testing.T) {
	app := setupApp()
	interceptor := &openAPIInterceptor{}
	app.Interceptor(interceptor)
	calls := 0
	resources := []boot.OpenAPIResource{{Path: "/custom-docs", ContentType: "text/plain", Body: "prepared documentation"}}
	provider := func(string) ([]boot.OpenAPIResource, error) {
		calls++
		if calls > 1 {
			resources[0] = boot.OpenAPIResource{Path: "/hello", ContentType: "text/plain", Body: "shadowed API"}
		}
		return resources, nil
	}
	h := newTestHandlerFromAppWithOptions(t, app, boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true, UI: []boot.OpenAPIUI{provider}}}})
	resources[0] = boot.OpenAPIResource{Path: "/changed", ContentType: "text/plain", Body: "changed after preparation"}
	if calls != 1 {
		t.Errorf("UI provider was called %d times during preparation; expected 1", calls)
	}
	for _, request := range []struct{ path, body string }{
		{"/custom-docs", "prepared documentation"},
		{"/hello", "hello"},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", request.path, nil))
		if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != request.body {
			t.Errorf("Validated resource snapshot changed: GET %s = %d %q", request.path, w.Code, w.Body.String())
		}
	}
	if interceptor.calls.Load() != 1 {
		t.Errorf("Business pipeline calls = %d; expected 1", interceptor.calls.Load())
	}
}

func TestOpenAPIUIRevalidatesProvidersForEachPreparation(t *testing.T) {
	app := setupApp()
	calls := 0
	options := boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true, UI: []boot.OpenAPIUI{func(string) ([]boot.OpenAPIResource, error) {
		calls++
		path := "/custom-docs"
		if calls > 1 {
			path = "/hello"
		}
		return []boot.OpenAPIResource{{Path: path, ContentType: "text/plain", Body: "documentation"}}, nil
	}}}}}
	if err := app.Validate(options); err != nil {
		t.Fatal(err)
	}
	prepared, err := app.Prepare(context.Background(), options)
	if prepared != nil {
		_ = prepared.Close(context.Background())
		t.Error("Preparation accepted a provider that now conflicts with a business route")
	}
	if err == nil || !strings.Contains(err.Error(), "OPENAPI_PATH_CONFLICT") {
		t.Fatalf("Preparation reused resources from an earlier validation: %v", err)
	}
}
