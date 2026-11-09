package httpplatform_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

var noContentFailure = errors.New("controller failure")

type noContentController struct{}

func (*noContentController) Empty() httpx.Response[map[string]any] {
	return httpx.Response[map[string]any]{Options: httpx.ResponseOptions{Status: http.StatusNoContent, Headers: map[string]string{"X-Test": "preserved"}}}
}

func (*noContentController) Populated() httpx.Response[map[string]any] {
	return httpx.Response[map[string]any]{Body: map[string]any{"name": "not sent"}, Options: httpx.ResponseOptions{Status: http.StatusNoContent, Cookies: []httpx.Cookie{{Name: "test", Value: "preserved"}}}}
}

func (*noContentController) OK() httpx.Response[map[string]any] {
	return httpx.Response[map[string]any]{Body: map[string]any{"name": "sent"}}
}

func (*noContentController) String() httpx.Response[string] {
	return httpx.Response[string]{Body: "not sent", Options: httpx.ResponseOptions{Status: http.StatusNoContent, Headers: map[string]string{"X-Test": "preserved"}}}
}

func (*noContentController) Binary() httpx.Binary {
	return httpx.Binary{ContentType: "application/octet-stream", Data: []byte("not sent"), Options: httpx.ResponseOptions{Status: http.StatusNoContent, Headers: map[string]string{"X-Test": "preserved"}}}
}

func (*noContentController) StringOK() httpx.Response[string] {
	return httpx.Response[string]{Body: "sent string"}
}

func (*noContentController) BinaryOK() httpx.Binary {
	return httpx.Binary{ContentType: "application/octet-stream", Data: []byte("sent binary")}
}

func (*noContentController) Failure() error { return noContentFailure }

type noContentProbe struct{ completed chan error }

func (*noContentProbe) PreHandle(core.ExecutionContext, core.HandlerMeta) error { return nil }
func (*noContentProbe) PostHandle(core.ExecutionContext, core.HandlerMeta)      {}
func (*noContentProbe) BeforeResponse(core.ExecutionContext, core.HandlerMeta, error) error {
	return nil
}
func (p *noContentProbe) AfterCompletion(_ core.ExecutionContext, _ core.HandlerMeta, err error) {
	p.completed <- err
}

func TestNoContentResponsesAndCompletionOverTCP(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		t.Run(engine, func(t *testing.T) {
			probe := &noContentProbe{completed: make(chan error, 1)}
			app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{})))
			app.Constructor(func() *noContentController { return &noContentController{} })
			app.InterceptorFor(boot.InterceptorHTTP, probe)
			app.Route("GET", "/empty", (*noContentController).Empty)
			app.Route("GET", "/populated", (*noContentController).Populated)
			app.Route("GET", "/ok", (*noContentController).OK)
			app.Route("GET", "/string", (*noContentController).String)
			app.Route("GET", "/binary", (*noContentController).Binary)
			app.Route("GET", "/string-ok", (*noContentController).StringOK)
			app.Route("GET", "/binary-ok", (*noContentController).BinaryOK)
			app.Route("GET", "/failure", (*noContentController).Failure)
			base, client := managedServer(t, prepare(t, app, boot.HTTPOptions{}))
			for _, tc := range []struct {
				path      string
				status    int
				body      string
				wantError error
			}{
				{"/empty", http.StatusNoContent, "", nil},
				{"/populated", http.StatusNoContent, "", nil},
				{"/ok", http.StatusOK, `{"name":"sent"}`, nil},
				{"/string", http.StatusNoContent, "", nil},
				{"/binary", http.StatusNoContent, "", nil},
				{"/string-ok", http.StatusOK, "sent string", nil},
				{"/binary-ok", http.StatusOK, "sent binary", nil},
				{"/failure", http.StatusInternalServerError, "", noContentFailure},
			} {
				t.Run(tc.path, func(t *testing.T) {
					resp, err := client.Get(base + tc.path)
					if err != nil {
						t.Fatal(err)
					}
					body, err := io.ReadAll(resp.Body)
					resp.Body.Close()
					if err != nil {
						t.Fatal(err)
					}
					if resp.StatusCode != tc.status || tc.wantError == nil && strings.TrimSpace(string(body)) != tc.body {
						t.Fatalf("status=%d body=%q", resp.StatusCode, body)
					}
					if (tc.path == "/empty" || tc.path == "/string" || tc.path == "/binary") && resp.Header.Get("X-Test") != "preserved" {
						t.Fatal(resp.Header)
					}
					if tc.path == "/populated" && !strings.Contains(resp.Header.Get("Set-Cookie"), "test=preserved") {
						t.Fatal(resp.Header)
					}
					select {
					case got := <-probe.completed:
						if !errors.Is(got, tc.wantError) {
							t.Fatalf("completion error=%v want=%v", got, tc.wantError)
						}
					case <-time.After(3 * time.Second):
						t.Fatal("completion timeout")
					}
				})
			}
		})
	}
}
