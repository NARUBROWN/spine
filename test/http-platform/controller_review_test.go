package httpplatform_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

type reviewController struct{}

func (*reviewController) Constructor() *reviewController { return &reviewController{} }
func (*reviewController) Legacy() httpx.Response[string] {
	return httpx.Response[string]{Body: "legacy"}
}
func (*reviewController) RegisterRoutes(r *api.Registry) {
	r.GET("/modern", func() httpx.Response[string] { return httpx.Response[string]{Body: "modern"} })
}

type handlerNameInterceptor struct{}

func (*handlerNameInterceptor) PreHandle(_ core.ExecutionContext, meta core.HandlerMeta) error {
	_ = meta.HandlerName()
	return nil
}
func (*handlerNameInterceptor) PostHandle(core.ExecutionContext, core.HandlerMeta) {}
func (*handlerNameInterceptor) BeforeResponse(core.ExecutionContext, core.HandlerMeta, error) error {
	return nil
}
func (*handlerNameInterceptor) AfterCompletion(core.ExecutionContext, core.HandlerMeta, error) {}

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
