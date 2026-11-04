package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type openAPIInterceptor struct{ calls atomic.Int32 }

func (i *openAPIInterceptor) PreHandle(core.ExecutionContext, core.HandlerMeta) error {
	i.calls.Add(1)
	return nil
}
func (*openAPIInterceptor) PostHandle(core.ExecutionContext, core.HandlerMeta) {}
func (*openAPIInterceptor) BeforeResponse(core.ExecutionContext, core.HandlerMeta, error) error {
	return nil
}
func (*openAPIInterceptor) AfterCompletion(core.ExecutionContext, core.HandlerMeta, error) {}

func TestOpenAPIIsOffWithoutExplicitActivation(t *testing.T) {
	for _, test := range []struct {
		name    string
		options boot.HTTPOptions
	}{
		{"omitted", boot.HTTPOptions{}},
		{"metadata_only", boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Path: "/internal/spec.json", Title: "Custom API", Version: "1"}}},
		{"inactive_invalid_path", boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Path: "relative"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			app := platform.New()
			app.Constructor(func() *appCtrl { return &appCtrl{} })
			app.Route("GET", "/hello", (*appCtrl).Hello)
			h := newTestHandlerFromAppWithOptions(t, app, boot.Options{HTTP: &test.options})
			for _, method := range []string{"GET", "HEAD"} {
				for _, path := range []string{"/openapi.json", "/internal/spec.json"} {
					w := httptest.NewRecorder()
					h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
					if w.Code != http.StatusNotFound {
						t.Fatalf("명시적으로 켜기 전에는 명세를 제공하면 안 됩니다: %s %s = %d", method, path, w.Code)
					}
				}
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/hello", nil))
			if w.Code != http.StatusOK {
				t.Fatalf("HTTP 앱은 계속 동작해야 합니다: %d", w.Code)
			}
		})
	}
}

func TestOpenAPIDefaultPreservesExistingRoutes(t *testing.T) {
	for _, test := range []struct {
		path, target, body string
		handler            any
	}{
		{"/openapi.json", "/openapi.json", "hello", (*appCtrl).Hello},
		{"/:id", "/42", "42", (*appCtrl).GetUser},
	} {
		t.Run(test.path, func(t *testing.T) {
			app := platform.New()
			app.Constructor(func() *appCtrl { return &appCtrl{} })
			app.Route("GET", test.path, test.handler)
			h := newTestHandlerFromAppWithOptions(t, app, boot.Options{HTTP: &boot.HTTPOptions{}})
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", test.target, nil))
			if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != test.body {
				t.Fatalf("기존 경로의 동작을 유지해야 합니다: %d %s", w.Code, w.Body)
			}
		})
	}
}

type openAPIDescriptionProbe struct{ describes atomic.Int32 }

func (*openAPIDescriptionProbe) Validate(context.Context, any) error {
	panic("준비 중 요청 검증을 실행하면 안 됩니다")
}

func (p *openAPIDescriptionProbe) DescribeSchema(reflect.Type) (validation.SchemaDescription, error) {
	p.describes.Add(1)
	return validation.SchemaDescription{}, nil
}

func TestOpenAPIGenerationOnlyRunsWhenEnabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		probe := &openAPIDescriptionProbe{}
		app := spine.New(platform.Option(), spine.WithValidator(probe))
		app.Constructor(func() *appCtrl { return &appCtrl{} })
		app.Route("POST", "/echo", (*appCtrl).Echo)
		h := newTestHandlerFromAppWithOptions(t, app, boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: enabled}}})
		if described := probe.describes.Load() > 0; described != enabled {
			t.Fatalf("명세 생성 여부가 설정과 달라졌습니다: enabled=%v calls=%d", enabled, probe.describes.Load())
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/openapi.json", nil))
		want := http.StatusNotFound
		if enabled {
			want = http.StatusOK
		}
		if w.Code != want {
			t.Fatalf("명세 제공 여부가 설정과 달라졌습니다: enabled=%v status=%d", enabled, w.Code)
		}
	}
}

func TestOpenAPIResourceUsesPreparedSnapshotAndSkipsBusinessInterceptors(t *testing.T) {
	app := setupApp()
	interceptor := &openAPIInterceptor{}
	app.Interceptor(interceptor)
	h := newTestHandlerFromAppWithOptions(t, app, boot.Options{HTTP: &boot.HTTPOptions{GlobalPrefix: "/api", OpenAPI: boot.OpenAPIOptions{Enabled: true}}})
	request := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w
	}
	first := request("GET", "/openapi.json")
	if first.Code != 200 || interceptor.calls.Load() != 0 {
		t.Fatalf("문서가 비즈니스 파이프라인을 실행했습니다: %d %d", first.Code, interceptor.calls.Load())
	}
	var document struct {
		OpenAPI string         `json:"openapi"`
		Paths   map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &document); err != nil || document.OpenAPI != "3.1.2" || document.Paths["/api/users/{id}"] == nil {
		t.Fatalf("문서: %s %v", first.Body, err)
	}
	app.Route("GET", "/after-prepare", (*appCtrl).Hello)
	second := request("GET", "/openapi.json")
	if second.Body.String() != first.Body.String() {
		t.Fatal("준비 후 등록 변경으로 명세가 바뀌었습니다")
	}
	head := request("HEAD", "/openapi.json")
	if head.Code != 200 || head.Body.Len() != 0 || head.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("HEAD: %+v", head)
	}
	if w := request("GET", "/api/hello"); w.Code != 200 || interceptor.calls.Load() != 1 {
		t.Fatalf("기존 실행: %d %d", w.Code, interceptor.calls.Load())
	}
}

func TestOpenAPIPathConfigurationAndCollision(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		app := setupApp()
		app.Route("GET", "/openapi.json", (*appCtrl).Hello)
		opts := boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: enabled}}}
		err := app.Validate(opts)
		if !enabled {
			if err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "OPENAPI_PATH_CONFLICT") {
			t.Fatalf("문서 경로 충돌을 거부해야 합니다: %v", err)
		}
	}
	app := setupApp()
	h := newTestHandlerFromAppWithOptions(t, app, boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true, Path: "/internal/spec.json", Title: "Custom API", Version: "1"}}})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/internal/spec.json", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"title":"Custom API"`) {
		t.Fatalf("설정 문서: %d %s", w.Code, w.Body)
	}
	app = platform.New()
	app.Constructor(func() *appCtrl { return &appCtrl{} })
	app.Route("GET", "/hello", (*appCtrl).Hello)
	h = newTestHandlerFromAppWithOptions(t, app, boot.Options{HTTP: &boot.HTTPOptions{}})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/openapi.json", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("비활성화 결과: %d", w.Code)
	}
}

func TestOpenAPIPreservesDuplicateRouteRejection(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		app := setupApp()
		app.Route("GET", "/repeat", (*appCtrl).Hello)
		app.Route("GET", "/repeat", (*appCtrl).Hello)
		prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: enabled}}})
		if err == nil {
			prepared.Close(t.Context())
			t.Fatalf("기존 중복 경로 거부 정책을 유지해야 합니다: enabled=%v", enabled)
		}
	}
}

func TestOpenAPIPathAlsoDetectsParameterRouteOverlap(t *testing.T) {
	app := setupApp()
	app.Route("GET", "/:id", (*appCtrl).GetUser)
	if err := app.Validate(boot.Options{HTTP: &boot.HTTPOptions{}}); err != nil {
		t.Fatalf("기본 비활성화 상태에서는 명세 경로를 예약하면 안 됩니다: %v", err)
	}
	if err := app.Validate(boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true}}}); err == nil || !strings.Contains(err.Error(), "OPENAPI_PATH_CONFLICT") {
		t.Fatalf("경로 인자의 명세 리소스 가림을 거부해야 합니다: %v", err)
	}
	if err := app.Validate(boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true, Path: "/internal/openapi.json"}}}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenAPIFromStandaloneExecutableWithoutSourcesOrCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("독립 실행 파일 빌드")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "app")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", binary, "./testdata/runtime-openapi")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("실행 파일 빌드: %v\n%s", err, output)
	}
	command := exec.Command(binary)
	command.Dir = dir
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "PATH=") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "PATH="+dir)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("실행 파일: %v", err)
	}
	var document struct {
		OpenAPI string         `json:"openapi"`
		Paths   map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(output, &document); err != nil || document.OpenAPI != "3.1.2" || document.Paths["/hello"] == nil {
		t.Fatalf("CLI와 소스 없는 실행: %s %v", output, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("실행 디렉터리: %+v %v", entries, err)
	}
}
