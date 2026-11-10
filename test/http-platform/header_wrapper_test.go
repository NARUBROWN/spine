package httpplatform_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	playground "github.com/NARUBROWN/spine-validator-playground"
	validation "github.com/NARUBROWN/spine/contract/validator"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
)

type wrappedHeaders struct {
	RequestID string `header:"X-Request-ID"`
	Token     string
	Ignored   string `header:"-"`
}
type legacyHeaders struct {
	RequestID string `header:"X-Request-ID"`
}
type headerController struct{}

func (*headerController) RegisterRoutes(r *api.Registry) {
	r.GET("/wrapped", func(h input.Header[wrappedHeaders]) httpx.Response[string] {
		return httpx.Response[string]{Body: h.RequestID + ":" + h.Token + ":" + h.Ignored}
	})
	r.GET("/legacy", func(h *legacyHeaders) httpx.Response[string] { return httpx.Response[string]{Body: h.RequestID} })
}
func TestHeaderWrapperAndLegacyHTTP(t *testing.T) {
	for _, adapter := range []string{"echo", "nethttp"} {
		t.Run(adapter, func(t *testing.T) {
			app := spine.New(spine.WithHTTPAdapter(platformFactory(adapter, nativeHooks{})), spine.WithValidator(&registeredSchemaValidator{}))
			app.Constructor(func() *headerController { return &headerController{} })
			prepared := prepare(t, app, boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true}})
			doc, err := spine.DescribePreparedHTTP(prepared)
			if err != nil {
				t.Fatal(err)
			}
			names := map[string]bool{}
			for _, route := range doc.Routes {
				if route.Path == "/wrapped" {
					for _, field := range route.Inputs {
						names[field.Binding+":"+field.Name] = true
					}
				}
			}
			if !names["header:X-Request-Id"] || !names["header:Token"] || names["header:Ignored"] {
				t.Fatalf("headers: %v", names)
			}
			base, client := managedServer(t, prepared)
			for _, tc := range []struct{ path, want string }{{"/wrapped", `abc:secret:`}, {"/legacy", `abc`}} {
				req, _ := http.NewRequest("GET", base+tc.path, nil)
				req.Header.Set("x-request-id", "abc")
				req.Header.Set("token", "secret")
				req.Header.Set("ignored", "bad")
				response, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != http.StatusOK || strings.TrimSpace(string(data)) != tc.want {
					t.Fatalf("%s: %d %s", tc.path, response.StatusCode, data)
				}
			}
		})
	}
}

type validatedHeaders struct {
	RequestID string `header:"X-Request-ID" validate:"required"`
}
type validatedHeaderController struct{}

func (*validatedHeaderController) RegisterRoutes(r *api.Registry) {
	r.GET("/required-header", func(h input.Header[validatedHeaders]) httpx.Response[string] {
		return httpx.Response[string]{Body: h.RequestID}
	})
}
func TestHeaderWrapperValidation(t *testing.T) {
	inner, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	validator := &recordedRegisteredValidator{inner: inner, errors: make(chan []validation.FieldError, 1)}
	app := spine.New(spine.WithHTTPAdapter(platformFactory("nethttp", nativeHooks{})), spine.WithValidator(validator))
	app.Constructor(func() *validatedHeaderController { return &validatedHeaderController{} })
	prepared := prepare(t, app, boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true}})
	base, client := managedServer(t, prepared)
	response, err := client.Get(base + "/required-header")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", response.StatusCode)
	}
	select {
	case fields := <-validator.errors:
		if len(fields) != 1 || fields[0].Field != "/X-Request-Id" {
			t.Fatalf("fields: %+v", fields)
		}
	default:
		t.Fatal("validation errors missing")
	}
	doc, err := spine.DescribePreparedHTTP(prepared)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, route := range doc.Routes {
		for _, field := range route.Inputs {
			if field.Binding == "header" && field.Name == "X-Request-Id" && field.Required {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("required header missing from description")
	}
}

type invalidHeaderCollision struct {
	First  string `header:"X-ID"`
	Second string `header:"x-id"`
}
type invalidHeaderController struct{ mode string }

func (c *invalidHeaderController) RegisterRoutes(r *api.Registry) {
	switch c.mode {
	case "scalar":
		r.GET("/invalid", func(input.Header[int]) error { return nil })
	case "duplicate":
		r.GET("/invalid", func(input.Header[invalidHeaderCollision]) error { return nil })
	}
}
func TestHeaderWrapperRejectsInvalidDeclarations(t *testing.T) {
	for _, mode := range []string{"scalar", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			app := spine.New(spine.WithHTTPAdapter(platformFactory("nethttp", nativeHooks{})))
			app.Constructor(func() *invalidHeaderController { return &invalidHeaderController{mode: mode} })
			prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{}})
			if err == nil {
				if prepared != nil {
					prepared.Close(t.Context())
				}
				t.Fatal("invalid header accepted")
			}
		})
	}
}
