package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/path"
)

type appCtrl struct{}

func (c *appCtrl) GetUser(id path.Int) httpx.Response[int] {
	return httpx.Response[int]{Body: int(id.Value)}
}

func (c *appCtrl) Hello() httpx.Response[string] {
	return httpx.Response[string]{Body: "hello"}
}

func (c *appCtrl) Fail() error {
	return httperr.BadRequest("bad")
}

func setupApp() spine.App {
	app := spine.New()
	app.Constructor(func() *appCtrl { return &appCtrl{} })
	app.Route("GET", "/users/:id", (*appCtrl).GetUser)
	app.Route("GET", "/hello", (*appCtrl).Hello)
	app.Route("GET", "/fail", (*appCtrl).Fail)
	return app
}

func newTestHandlerFromApp(t *testing.T, app spine.App) http.Handler {
	t.Helper()

	ready := make(chan http.Handler, 1)
	runErr := make(chan error, 1)

	app.Transport(func(v any) {
		h, ok := v.(http.Handler)
		if !ok {
			return
		}
		select {
		case ready <- h:
		default:
		}
	})

	go func() {
		runErr <- app.Run(boot.Options{
			Address:                "127.0.0.1:0",
			EnableGracefulShutdown: true,
			HTTP:                   &boot.HTTPOptions{},
		})
	}()

	var h http.Handler
	select {
	case h = <-ready:
	case err := <-runErr:
		t.Fatalf("spine 앱 실행 실패: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatalf("spine 앱 시작 타임아웃")
	}

	t.Cleanup(func() {
		stopped := false
		select {
		case <-runErr:
			stopped = true
		default:
		}

		if !stopped {
			if p, err := os.FindProcess(os.Getpid()); err == nil {
				_ = p.Signal(os.Interrupt)
			}

			select {
			case <-runErr:
			case <-time.After(3 * time.Second):
				t.Fatalf("spine 앱 종료 타임아웃")
			}
		}
	})

	return h
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
