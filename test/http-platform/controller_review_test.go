package httpplatform_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
	handlerspine "github.com/NARUBROWN/spine/pkg/spine"
)

type reviewController struct{}

func (*reviewController) Constructor() *reviewController { return &reviewController{} }
func (*reviewController) Legacy() httpx.Response[string] {
	return httpx.Response[string]{Body: "legacy"}
}
func (*reviewController) RegisterRoutes(r *api.Registry) {
	r.GET("/modern", func(ctx handlerspine.Ctx) httpx.Response[string] {
		value, _ := ctx.ExecutionContext().Get("review")
		return httpx.Response[string]{Body: value.(string)}
	})
}

type handlerNameInterceptor struct{}

func (*handlerNameInterceptor) PreHandle(ctx handlerspine.Ctx, meta core.HandlerMeta) error {
	_ = meta.HandlerName()
	ctx.ExecutionContext().Set("review", "modern")
	return nil
}
func (*handlerNameInterceptor) PostHandle(handlerspine.Ctx, core.HandlerMeta) {}
func (*handlerNameInterceptor) BeforeResponse(handlerspine.Ctx, core.HandlerMeta, error) error {
	return nil
}
func (*handlerNameInterceptor) AfterCompletion(handlerspine.Ctx, core.HandlerMeta, error) {}

func TestLegacyAndControllerRoutesCoexistWithGlobalHandlerNames(t *testing.T) {
	for _, platform := range []string{"echo", "nethttp"} {
		t.Run(platform, func(t *testing.T) {
			app := spine.New(spine.WithHTTPAdapter(platformFactory(platform, nativeHooks{})))
			app.Constructor(
				constructor.Custom(func() api.Controller { panic("excluded factory must not execute") }),
				constructor.Controller(&reviewController{}),
			)
			app.Route("GET", "/legacy", (*reviewController).Legacy)
			app.InterceptorFor(boot.InterceptorHTTP, &handlerNameInterceptor{})
			if err := app.Validate(boot.Options{HTTP: &boot.HTTPOptions{}}); err != nil {
				t.Fatal(err)
			}
			p := prepare(t, app, boot.HTTPOptions{})
			base, client := managedServer(t, p)
			for _, path := range []string{"legacy", "modern"} {
				response, err := client.Get(base + "/" + path)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil || response.StatusCode != http.StatusOK || string(body) != path {
					t.Fatalf("path=%s status=%d body=%s err=%v", path, response.StatusCode, body, err)
				}
			}
			description, err := spine.DescribePreparedHTTP(p)
			if err != nil || len(description.Routes) != 2 {
				t.Fatalf("routes=%+v err=%v", description.Routes, err)
			}
			app.Route("GET", "/modern", (*reviewController).Legacy)
			duplicate, err := app.Prepare(context.Background(), boot.Options{HTTP: &boot.HTTPOptions{}})
			if err == nil {
				duplicate.Close(context.Background())
				t.Fatal("duplicate legacy and controller route accepted")
			}
			if !strings.Contains(err.Error(), "/modern") {
				t.Fatal(err)
			}
		})
	}
}

type boundaryController struct{}

func (c *boundaryController) RegisterRoutes(r *api.Registry) {
	r.GET("/boundary/:id", func(id input.Path[string]) httpx.Response[string] {
		return httpx.Response[string]{Body: *id}
	}, api.Interceptors(&boundaryInterceptor{}))
}

type boundaryInterceptor struct{ handlerNameInterceptor }

func (*boundaryInterceptor) PreHandle(ctx handlerspine.Ctx, meta core.HandlerMeta) error {
	for _, name := range []string{"SetRoute", "SetResponseWriter", "EventBus"} {
		if _, ok := reflect.TypeOf(ctx).MethodByName(name); ok {
			return fmt.Errorf("public interceptor exposes %s", name)
		}
	}
	if len(meta.PathKeys) != 1 || meta.PathKeys[0] != "id" || meta.FunctionType() == nil {
		return fmt.Errorf("unexpected route metadata: %+v", meta)
	}
	meta.PathKeys[0] = "changed"
	return nil
}

func TestPublicInterceptorMetadataCannotChangeRouteBinding(t *testing.T) {
	for _, platform := range []string{"echo", "nethttp"} {
		t.Run(platform, func(t *testing.T) {
			app := spine.New(spine.WithHTTPAdapter(platformFactory(platform, nativeHooks{})))
			app.Constructor(func() *boundaryController { return &boundaryController{} })
			prepared := prepare(t, app, boot.HTTPOptions{})
			base, client := managedServer(t, prepared)
			for _, id := range []string{"first", "second"} {
				response, err := client.Get(base + "/boundary/" + id)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil || response.StatusCode != http.StatusOK || string(body) != id {
					t.Fatalf("status=%d body=%s err=%v", response.StatusCode, body, err)
				}
			}
		})
	}
}
