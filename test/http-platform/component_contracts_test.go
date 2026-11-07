package httpplatform_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

type roleCounters struct{ builds, requests atomic.Int32 }

func (*roleCounters) RegisterRoutes(*api.Registry) {
	panic("a custom factory must not be collected as a controller")
}

type roleCustom struct {
	generation int32
	counters   *roleCounters
}

func (*roleCustom) Constructor(c *roleCounters) *roleCustom {
	return &roleCustom{generation: c.builds.Add(1), counters: c}
}
func (*roleCustom) RegisterRoutes(*api.Registry) {
	panic("a custom object must not be collected as a controller")
}

type roleRepository struct{ custom *roleCustom }

func (*roleRepository) Constructor(c *roleCustom) *roleRepository { return &roleRepository{c} }

type roleService struct{ repo *roleRepository }

func (*roleService) Constructor(r *roleRepository) *roleService { return &roleService{r} }
func (*roleService) RegisterRoutes(*api.Registry) {
	panic("a service must not be collected as a controller")
}

type roleController struct{ service *roleService }

func (*roleController) Constructor(s *roleService) *roleController { return &roleController{s} }
func (c *roleController) RegisterRoutes(r *api.Registry) {
	r.GET("/role", func() httpx.Response[int32] { return httpx.Response[int32]{Body: c.service.repo.custom.generation} })
}

type roleInterceptor struct{ counters *roleCounters }

func (*roleInterceptor) Constructor(c *roleCounters) *roleInterceptor { return &roleInterceptor{c} }
func (i *roleInterceptor) PreHandle(core.ExecutionContext, core.HandlerMeta) error {
	i.counters.requests.Add(1)
	return nil
}
func (*roleInterceptor) PostHandle(core.ExecutionContext, core.HandlerMeta) {}
func (*roleInterceptor) BeforeResponse(core.ExecutionContext, core.HandlerMeta, error) error {
	return nil
}
func (*roleInterceptor) AfterCompletion(core.ExecutionContext, core.HandlerMeta, error) {}

func TestRoleConstructorsHTTPAndPreparedIsolation(t *testing.T) {
	for _, platform := range []string{"echo", "nethttp"} {
		t.Run(platform, func(t *testing.T) {
			counters := &roleCounters{}
			app := spine.New(spine.WithHTTPAdapter(platformFactory(platform, nativeHooks{})))
			app.Constructor(
				constructor.Custom(func() *roleCounters { return counters }),
				constructor.Custom(&roleCustom{}),
				constructor.Repository(&roleRepository{}),
				constructor.Service(&roleService{}),
				constructor.Controller(&roleController{}),
				constructor.Interceptor(&roleInterceptor{}),
			)
			options := boot.Options{HTTP: &boot.HTTPOptions{}}
			if err := app.Validate(options); err != nil {
				t.Fatal(err)
			}
			if counters.builds.Load() != 0 {
				t.Fatal("validation invoked a constructor")
			}
			for index := int32(1); index <= 2; index++ {
				p, err := app.Prepare(context.Background(), options)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { p.Close(context.Background()) })
				base, client := managedServer(t, p)
				response, err := client.Get(base + "/role")
				if err != nil {
					t.Fatal(err)
				}
				var value int32
				err = json.NewDecoder(response.Body).Decode(&value)
				response.Body.Close()
				if err != nil || response.StatusCode != http.StatusOK || value != index {
					t.Fatalf("response=%d value=%d err=%v", response.StatusCode, value, err)
				}
			}
			if counters.requests.Load() != 0 {
				t.Fatal("registering an interceptor applied it globally")
			}
			app.InterceptorFor(boot.InterceptorHTTP, (*roleInterceptor)(nil))
			p, err := app.Prepare(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { p.Close(context.Background()) })
			base, client := managedServer(t, p)
			response, err := client.Get(base + "/role")
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if counters.requests.Load() != 1 || counters.builds.Load() != 3 {
				t.Fatalf("builds=%d requests=%d", counters.builds.Load(), counters.requests.Load())
			}
		})
	}
}
