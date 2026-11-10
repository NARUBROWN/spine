package test

import httpinput "github.com/NARUBROWN/spine/pkg/input"

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

type userPath struct {
	ID int64 `path:"id"`
}
type appCtrl struct{}

func (c *appCtrl) GetUser(p httpinput.Path[userPath]) httpx.Response[int] {
	return httpx.Response[int]{Body: int(p.ID)}
}

func (c *appCtrl) Hello() httpx.Response[string] {
	return httpx.Response[string]{Body: "hello"}
}

func (c *appCtrl) Fail() error {
	return httperr.BadRequest("bad")
}

func (c *appCtrl) Echo(req httpinput.Body[echoRequest]) httpx.Response[string] {
	return httpx.Response[string]{Body: req.Name}
}

type echoRequest struct {
	Name string `json:"name"`
}

func setupApp() spine.App {
	app := platform.New()
	app.Constructor(func() *appCtrl { return &appCtrl{} })
	app.Route("GET", "/users/:id", (*appCtrl).GetUser)
	app.Route("GET", "/hello", (*appCtrl).Hello)
	app.Route("GET", "/fail", (*appCtrl).Fail)
	app.Route("POST", "/echo", (*appCtrl).Echo)
	return app
}

func newTestHandlerFromApp(t *testing.T, app spine.App) http.Handler {
	return newTestHandlerFromAppWithOptions(t, app, boot.Options{
		Address:                "127.0.0.1:0",
		EnableGracefulShutdown: true,
		HTTP:                   &boot.HTTPOptions{},
	})
}

func newTestHandlerFromAppWithOptions(t *testing.T, app spine.App, opts boot.Options) http.Handler {
	t.Helper()

	if opts.HTTP == nil {
		opts.HTTP = &boot.HTTPOptions{}
	}
	prepared, err := app.Prepare(context.Background(), opts)
	if err != nil {
		t.Fatalf("Spine 앱 준비 실패: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := prepared.Close(ctx); err != nil {
			t.Errorf("Spine 앱 종료 실패: %v", err)
		}
	})
	return mustHTTPHandler(t, prepared)
}

func TestAppIntegration_JSON(t *testing.T) {
	app := setupApp()
	handler := newTestHandlerFromApp(t, app)

	req := httptest.NewRequest("GET", "/users/7", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("상태 코드가 잘못되었습니다: %d", resp.StatusCode)
	}

	var body int
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("바디 파싱 실패: %v", err)
	}
	if body != 7 {
		t.Fatalf("응답 값이 잘못되었습니다: %d", body)
	}
}

func TestAppIntegration_String(t *testing.T) {
	app := setupApp()
	handler := newTestHandlerFromApp(t, app)

	req := httptest.NewRequest("GET", "/hello", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("상태 코드가 잘못되었습니다: %d", resp.StatusCode)
	}

	body := rec.Body.String()
	if body != "hello" {
		t.Fatalf("문자열 응답이 잘못되었습니다: %q", body)
	}
}

func TestAppIntegration_Error(t *testing.T) {
	app := setupApp()
	handler := newTestHandlerFromApp(t, app)

	req := httptest.NewRequest("GET", "/fail", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("상태 코드가 잘못되었습니다: %d", resp.StatusCode)
	}

	var parsed map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		t.Fatalf("JSON 파싱 실패: %v", err)
	}

	if parsed["message"] != "bad" {
		t.Fatalf("에러 메시지가 잘못되었습니다: %v", parsed)
	}
}

func TestAppIntegration_BodyLimit(t *testing.T) {
	app := setupApp()
	handler := newTestHandlerFromAppWithOptions(t, app, boot.Options{
		Address:                "127.0.0.1:0",
		EnableGracefulShutdown: true,
		HTTP: &boot.HTTPOptions{
			MaxBodyBytes: 16,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"name":"payload too large"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("상태 코드는 413이어야 합니다. 실제=%d", resp.StatusCode)
	}
}
