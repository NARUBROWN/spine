package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	platform "github.com/NARUBROWN/spine-echo-platform"
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

func TestOpenAPIResourceUsesPreparedSnapshotAndSkipsBusinessInterceptors(t *testing.T) {
	app := setupApp()
	interceptor := &openAPIInterceptor{}
	app.Interceptor(interceptor)
	h := newTestHandlerFromAppWithOptions(t, app, boot.Options{HTTP: &boot.HTTPOptions{GlobalPrefix: "/api"}})
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
	for _, disabled := range []bool{false, true} {
		app := setupApp()
		app.Route("GET", "/openapi.json", (*appCtrl).Hello)
		opts := boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Disabled: disabled}}}
		err := app.Validate(opts)
		if disabled {
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
	h := newTestHandlerFromAppWithOptions(t, app, boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Path: "/internal/spec.json", Title: "Custom API", Version: "1"}}})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/internal/spec.json", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"title":"Custom API"`) {
		t.Fatalf("설정 문서: %d %s", w.Code, w.Body)
	}
	app = platform.New()
	app.Constructor(func() *appCtrl { return &appCtrl{} })
	app.Route("GET", "/hello", (*appCtrl).Hello)
	h = newTestHandlerFromAppWithOptions(t, app, boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Disabled: true}}})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/openapi.json", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("비활성화 결과: %d", w.Code)
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
		t.Fatalf("CLI·소스 없는 실행: %s %v", output, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("실행 디렉터리: %+v %v", entries, err)
	}
}
