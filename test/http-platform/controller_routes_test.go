package httpplatform_test

import (
	"context"
	"encoding/json"
	"errors"
	playground "github.com/NARUBROWN/spine-validator-playground"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
)

type controllerPath struct{ ID int }
type controllerQuery struct {
	UserID int
	Tags   []string
	Hidden string `query:"-"`
}
type controllerBody struct {
	Name string `json:"name"`
}
type callbackResponse = httpx.Response[map[string]any]
type registeredController struct {
	generation    int
	registrations *atomic.Int32
	mode          string
}

var _ api.Controller = (*registeredController)(nil)

func (c *registeredController) RegisterRoutes(r *api.Registry) {
	c.registrations.Add(1)
	switch c.mode {
	case "panic":
		panic("broken controller")
	case "error":
		r.Error(errors.New("missing token"))
		return
	case "nil":
		r.POST("/users", (func(input.Body[controllerBody]) error)(nil))
		return
	case "payload":
		r.POST("/users", func(input.Body[int]) error { return nil })
		return
	case "return":
		r.POST("/users", func() int { return 1 })
		return
	case "body":
		r.POST("/users", func(input.Body[controllerBody], input.Body[bodyDTO]) error { return nil })
		return
	case "collision":
		r.GET("/openapi.json", func() error { return nil })
		return
	}
	r.POST("/users/:id", func(a input.Body[controllerBody], ctx context.Context, p input.Path[controllerPath], q input.Query[controllerQuery]) (callbackResponse, error) {
		return callbackResponse{Body: map[string]any{"name": c.normalize(a.Name), "id": p.ID, "userId": q.UserID, "tags": q.Tags, "generation": c.generation, "context": ctx != nil}}, nil
	})
	c.registerAux(r)
}
func (c *registeredController) normalize(name string) string { return strings.TrimSpace(name) }
func (c *registeredController) registerAux(r *api.Registry) {
	r.GET("/generation", func() httpx.Response[int] { return httpx.Response[int]{Body: c.generation} })
}

func TestControllerRoutesTCPAndPreparedSnapshots(t *testing.T) {
	for _, name := range []string{"echo", "nethttp"} {
		t.Run(name, func(t *testing.T) {
			app := spine.New(spine.WithHTTPAdapter(platformFactory(name, nativeHooks{})))
			var calls, registrations atomic.Int32
			app.Constructor(func() *registeredController {
				return &registeredController{generation: int(calls.Add(1)), registrations: &registrations}
			})
			options := boot.Options{HTTP: &boot.HTTPOptions{GlobalPrefix: "/v1"}}
			if _, err := spine.DescribeHTTP(app, options); !errors.Is(err, spine.ErrRoutesNotCollected) {
				t.Fatalf("unprepared description: %v", err)
			}
			if calls.Load() != 0 || registrations.Load() != 0 {
				t.Fatal("description executed user construction")
			}
			first := prepare(t, app, *options.HTTP)
			second := prepare(t, app, *options.HTTP)
			if calls.Load() != 2 || registrations.Load() != 2 {
				t.Fatalf("constructors=%d registrations=%d", calls.Load(), registrations.Load())
			}
			for index, p := range []spine.PreparedApplication{first, second} {
				base, client := managedServer(t, p)
				response, err := client.Post(base+"/v1/users/7?userId=8&tags=a&tags=b", "application/json", strings.NewReader(`{"name":" Alice "}`))
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				var got map[string]any
				if err := json.Unmarshal(data, &got); err != nil {
					t.Fatalf("%d %s: %v", response.StatusCode, data, err)
				}
				if response.StatusCode != 200 || got["id"] != float64(7) || got["userId"] != float64(8) || got["name"] != "Alice" || got["generation"] != float64(index+1) || got["context"] != true {
					t.Fatalf("%d %s", response.StatusCode, data)
				}
				for n := 0; n < 2; n++ {
					doc, err := spine.DescribePreparedHTTP(p)
					if err != nil {
						t.Fatal(err)
					}
					if len(doc.Routes) != 2 {
						t.Fatalf("routes=%+v", doc.Routes)
					}
					doc.Routes[0].Path = "changed"
				}
			}
			if registrations.Load() != 2 {
				t.Fatal("requests or descriptions repeated registration")
			}
		})
	}
}

func TestControllerRoutesFailBeforeAdapterAndIgnoreHTTPDisabled(t *testing.T) {
	for _, mode := range []string{"panic", "error", "nil", "payload", "return", "body", "collision"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			app := spine.New(spine.WithHTTPAdapter(platformFactory("nethttp", nativeHooks{})))
			app.Constructor(func() *registeredController { return &registeredController{mode: mode, registrations: &calls} })
			p, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true}}})
			if err == nil || p != nil {
				if p != nil {
					p.Close(context.Background())
				}
				t.Fatalf("mode %s succeeded", mode)
			}
		})
	}
	var calls atomic.Int32
	app := spine.New()
	app.Constructor(func() *registeredController { calls.Add(1); panic("HTTP disabled") })
	p, err := app.Prepare(t.Context(), boot.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close(context.Background())
	if calls.Load() != 0 {
		t.Fatal("HTTP disabled constructed controller")
	}
}

type registeredSchemaValidator struct{}

func (*registeredSchemaValidator) Validate(context.Context, any) error { return nil }
func (*registeredSchemaValidator) DescribeSchema(reflect.Type) (validation.SchemaDescription, error) {
	return validation.SchemaDescription{}, nil
}

func TestControllerRoutesDescriptionMatchesInputPlan(t *testing.T) {
	for _, mode := range []string{"none", "validator", "schema"} {
		t.Run(mode, func(t *testing.T) {
			options := []spine.Option{spine.WithHTTPAdapter(platformFactory("nethttp", nativeHooks{}))}
			if mode == "validator" {
				options = append(options, spine.WithValidator(&inputValidator{}))
			}
			if mode == "schema" {
				options = append(options, spine.WithValidator(&registeredSchemaValidator{}))
			}
			app := spine.New(options...)
			var registrations atomic.Int32
			app.Constructor(func() *registeredController { return &registeredController{registrations: &registrations} })
			prepared := prepare(t, app, boot.HTTPOptions{GlobalPrefix: "/v1", OpenAPI: boot.OpenAPIOptions{Enabled: true}})
			doc, err := spine.DescribePreparedHTTP(prepared)
			if err != nil {
				t.Fatal(err)
			}
			if doc.PathsOnly != (mode == "none") {
				t.Fatalf("PathsOnly=%v mode=%s", doc.PathsOnly, mode)
			}
			if mode == "schema" {
				names := map[string]bool{}
				body := false
				for _, route := range doc.Routes {
					if route.Method != "POST" {
						continue
					}
					for _, input := range route.Inputs {
						names[input.Binding+":"+input.Name] = true
						if input.Binding == "body" {
							body = len(input.Content) > 0
						}
					}
				}
				if !body || !names["path:id"] || !names["query:userId"] || !names["query:tags"] || names["query:hidden"] {
					t.Fatalf("names=%v body=%v", names, body)
				}
			}
			for count := 0; count < 2; count++ {
				if _, err := spine.DescribePreparedHTTP(prepared); err != nil {
					t.Fatal(err)
				}
			}
			if registrations.Load() != 1 {
				t.Fatal("description repeated registration")
			}
		})
	}
}

func TestControllerInterfaceProviderFailsWithoutConstruction(t *testing.T) {
	var calls atomic.Int32
	app := spine.New(spine.WithHTTPAdapter(platformFactory("nethttp", nativeHooks{})))
	app.Constructor(func() api.Controller { calls.Add(1); return &registeredController{registrations: &calls} })
	options := boot.Options{HTTP: &boot.HTTPOptions{}}
	if err := app.Validate(options); err == nil || !strings.Contains(err.Error(), "HTTP_CONTROLLER_PROVIDER_ABSTRACT") {
		t.Fatalf("validation=%v", err)
	}
	if _, err := spine.DescribeHTTP(app, options); err == nil {
		t.Fatal("abstract provider description accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("pure validation executed constructor")
	}
}

type RegisteredValidationEmbedded struct {
	UserID int `validate:"required"`
}
type registeredValidationQuery struct {
	RegisteredValidationEmbedded
	Alias int `query:"UserID" validate:"required"`
}
type validatedRegisteredController struct{}

func (*validatedRegisteredController) RegisterRoutes(r *api.Registry) {
	r.GET("/validated", func(q input.Query[registeredValidationQuery]) httpx.Response[int] {
		return httpx.Response[int]{Body: q.UserID + q.Alias}
	})
}

type recordedRegisteredValidator struct {
	inner  *playground.Validator
	errors chan []validation.FieldError
}

func (v *recordedRegisteredValidator) Validate(ctx context.Context, dto any) error {
	err := v.inner.Validate(ctx, dto)
	if details, ok := validation.AsError(err); ok {
		v.errors <- append([]validation.FieldError(nil), details.Fields...)
	}
	return err
}
func (v *recordedRegisteredValidator) DescribeSchema(t reflect.Type) (validation.SchemaDescription, error) {
	return v.inner.DescribeSchema(t)
}

func TestControllerInputNamesReachActualValidatorAndSchema(t *testing.T) {
	inner, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	validator := &recordedRegisteredValidator{inner: inner, errors: make(chan []validation.FieldError, 1)}
	app := spine.New(spine.WithHTTPAdapter(platformFactory("nethttp", nativeHooks{})), spine.WithValidator(validator))
	app.Constructor(func() *validatedRegisteredController { return &validatedRegisteredController{} })
	prepared := prepare(t, app, boot.HTTPOptions{})
	base, client := managedServer(t, prepared)
	response, err := client.Get(base + "/validated")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatal(response.StatusCode)
	}
	select {
	case fields := <-validator.errors:
		if len(fields) != 2 || fields[0].Field != "/UserID" || fields[1].Field != "/userId" {
			t.Fatal(fields)
		}
	default:
		t.Fatal("validator field errors not recorded")
	}
	doc, err := spine.DescribePreparedHTTP(prepared)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, route := range doc.Routes {
		for _, input := range route.Inputs {
			names[input.Name] = input.Required
		}
	}
	if !names["UserID"] || !names["userId"] {
		t.Fatalf("schema=%v", names)
	}
	response, err = client.Get(base + "/validated?userId=3&UserID=4")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 || strings.TrimSpace(string(data)) != "7" {
		t.Fatalf("%d %s", response.StatusCode, data)
	}
}
