package test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/labstack/echo/v5"
)

type preparationController struct{ generation int }
type preparationContextKey struct{}
type preparationMissingConsumer struct{}

func (*preparationMissingConsumer) Handle() {}
func (c *preparationController) Generation() httpx.Response[int] {
	return httpx.Response[int]{Body: c.generation}
}
func (*preparationController) Context(ctx context.Context) httpx.Response[string] {
	value, _ := ctx.Value(preparationContextKey{}).(string)
	return httpx.Response[string]{Body: value}
}

func prepareForHTTPTest(t *testing.T, app spine.App, ctx context.Context, options boot.Options) spine.PreparedApplication {
	t.Helper()
	prepared, err := app.Prepare(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := prepared.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return prepared
}

func assertPreparedHTTP(t *testing.T, handler http.Handler, path string, status int, body string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
	if recorder.Code != status || (body != "" && strings.TrimSpace(recorder.Body.String()) != body) {
		t.Fatalf("%s = %d %s", path, recorder.Code, recorder.Body.String())
	}
}

func TestAppIntegration_PrepareCreatesIndependentDIAndRouteSnapshots(t *testing.T) {
	app := platform.New()
	var generations int
	app.Constructor(func() *preparationController { generations++; return &preparationController{generation: generations} })
	app.Route("GET", "/generation", (*preparationController).Generation)
	options := boot.Options{HTTP: &boot.HTTPOptions{GlobalPrefix: "/first"}}
	first := prepareForHTTPTest(t, app, context.Background(), options)
	options.HTTP.GlobalPrefix = "/second"
	app.Route("GET", "/later", (*preparationController).Generation)
	second := prepareForHTTPTest(t, app, context.Background(), options)
	assertPreparedHTTP(t, mustHTTPHandler(t, first), "/first/generation", 200, "1")
	assertPreparedHTTP(t, mustHTTPHandler(t, first), "/first/later", 404, "")
	assertPreparedHTTP(t, mustHTTPHandler(t, first), "/second/generation", 404, "")
	assertPreparedHTTP(t, mustHTTPHandler(t, second), "/second/generation", 200, "2")
	assertPreparedHTTP(t, mustHTTPHandler(t, second), "/second/later", 200, "2")
	if err := first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertPreparedHTTP(t, mustHTTPHandler(t, second), "/second/generation", 200, "2")
}

func TestAppIntegration_PrepareFreezesConsumerRegistrationsBeforeNativeHooks(t *testing.T) {
	var app spine.App
	var register sync.Once
	app = platform.New(platform.WithEcho(func(_ *echo.Echo) {
		register.Do(func() {
			if err := app.Consumers().Register("later", (*preparationMissingConsumer).Handle); err != nil {
				t.Error(err)
			}
		})
	}))
	app.Constructor(func() *preparationController { return &preparationController{generation: 1} })
	app.Route("GET", "/generation", (*preparationController).Generation)
	app.Consumers()

	options := boot.Options{HTTP: &boot.HTTPOptions{}}
	first := prepareForHTTPTest(t, app, context.Background(), options)
	if len(app.Consumers().Registrations()) != 1 {
		t.Fatal("원본 앱의 등록 변경은 다음 준비에 반영돼야 합니다")
	}
	if prepared, err := app.Prepare(context.Background(), options); err == nil || prepared != nil {
		if prepared != nil {
			_ = prepared.Close(context.Background())
		}
		t.Fatal("다음 준비에서는 새 consumer의 누락된 DI를 검사해야 합니다")
	}
	assertPreparedHTTP(t, mustHTTPHandler(t, first), "/generation", 200, "1")
}

func TestAppIntegration_PrepareContextEndsBeforeHTTPHandlerLifetime(t *testing.T) {
	app := platform.New()
	app.Constructor(func() *preparationController { return &preparationController{} })
	app.Route("GET", "/context", (*preparationController).Context)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), preparationContextKey{}, "prepare"))
	defer cancel()
	prepared := prepareForHTTPTest(t, app, ctx, boot.Options{HTTP: &boot.HTTPOptions{}})
	cancel()
	request := httptest.NewRequest("GET", "/context", nil)
	request = request.WithContext(context.WithValue(request.Context(), preparationContextKey{}, "request"))
	recorder := httptest.NewRecorder()
	mustHTTPHandler(t, prepared).ServeHTTP(recorder, request)
	if recorder.Code != 200 || strings.TrimSpace(recorder.Body.String()) != "request" {
		t.Fatalf("준비 context와 요청 context를 분리해야 합니다: %d %s", recorder.Code, recorder.Body.String())
	}
}
