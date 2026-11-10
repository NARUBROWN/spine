package httpplatform_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/interceptor/cors"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/route"
	contextapi "github.com/NARUBROWN/spine/pkg/spine"
)

type corsProbe struct{ calls atomic.Int32 }

func (p *corsProbe) Resource() httpx.Response[string] {
	p.calls.Add(1)
	return httpx.Response[string]{Body: "ok"}
}
func (p *corsProbe) Failure() error { p.calls.Add(1); return errors.New("probe failure") }
func (p *corsProbe) Options() httpx.Response[string] {
	p.calls.Add(1)
	return httpx.Response[string]{Body: "options"}
}

type varyProbe struct{}

func (*varyProbe) PreHandle(ctx contextapi.Ctx, _ core.HandlerMeta) error {
	httpView, _ := ctx.HTTP()
	httpView.Response().SetHeader("Vary", "Accept-Encoding")
	return nil
}
func (*varyProbe) PostHandle(contextapi.Ctx, core.HandlerMeta)                  {}
func (*varyProbe) BeforeResponse(contextapi.Ctx, core.HandlerMeta, error) error { return nil }
func (*varyProbe) AfterCompletion(contextapi.Ctx, core.HandlerMeta, error)      {}

type corsAuthProbe struct{ calls atomic.Int32 }

func (p *corsAuthProbe) PreHandle(_ contextapi.Ctx, _ core.HandlerMeta) error {
	p.calls.Add(1)
	return nil
}
func (*corsAuthProbe) PostHandle(contextapi.Ctx, core.HandlerMeta) {}
func (*corsAuthProbe) BeforeResponse(contextapi.Ctx, core.HandlerMeta, error) error {
	return nil
}
func (*corsAuthProbe) AfterCompletion(contextapi.Ctx, core.HandlerMeta, error) {}

func TestCORSPolicyRealHTTP(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		t.Run(engine, func(t *testing.T) {
			probe, auth := &corsProbe{}, &corsAuthProbe{}
			age := 60
			policy, err := cors.NewValidated(cors.Config{AllowOrigins: []string{"https://app.example"}, AllowMethods: []string{"GET"}, AllowHeaders: []string{"X-Request-ID"}, AllowCredentials: true, ExposeHeaders: []string{"X-Response-ID"}, MaxAge: &age})
			if err != nil {
				t.Fatal(err)
			}
			app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{})))
			app.Constructor(func() *corsProbe { return probe })
			app.InterceptorFor(boot.InterceptorHTTP, &varyProbe{}, policy, auth)
			app.Route("GET", "/resource", (*corsProbe).Resource)
			app.Route("OPTIONS", "/resource", (*corsProbe).Options)
			app.Route("GET", "/failure", (*corsProbe).Failure)
			base, client := managedServer(t, prepare(t, app, boot.HTTPOptions{}))
			tests := []struct {
				name, method, origin, preflightMethod, preflightHeaders string
				status                                                  int
				allow                                                   bool
				authCount                                               int32
			}{
				{"allowed", "OPTIONS", "https://app.example", "GET", "x-request-id", 204, true, 0},
				{"method denied", "OPTIONS", "https://app.example", "DELETE", "x-request-id", 403, false, 0},
				{"header denied", "OPTIONS", "https://app.example", "GET", "x-bad", 403, false, 0},
				{"origin denied", "OPTIONS", "https://evil.example", "GET", "", 403, false, 0},
				{"ordinary options", "OPTIONS", "", "", "", 200, false, 1},
				{"normal allowed", "GET", "https://app.example", "", "", 200, true, 2},
				{"normal denied", "GET", "https://evil.example", "", "", 200, false, 3},
			}
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					req, err := http.NewRequest(tc.method, base+"/resource", nil)
					if err != nil {
						t.Fatal(err)
					}
					if tc.origin != "" {
						req.Header.Set("Origin", tc.origin)
					}
					if tc.preflightMethod != "" {
						req.Header.Set("Access-Control-Request-Method", tc.preflightMethod)
					}
					if tc.preflightHeaders != "" {
						req.Header.Set("Access-Control-Request-Headers", tc.preflightHeaders)
					}
					resp, err := client.Do(req)
					if err != nil {
						t.Fatal(err)
					}
					defer resp.Body.Close()
					_, _ = io.ReadAll(resp.Body)
					if resp.StatusCode != tc.status || auth.calls.Load() != tc.authCount {
						t.Fatalf("status=%d auth=%d", resp.StatusCode, auth.calls.Load())
					}
					vary := strings.ToLower(strings.Join(resp.Header.Values("Vary"), ","))
					if !strings.Contains(vary, "accept-encoding") || !strings.Contains(vary, "origin") {
						t.Fatal(vary)
					}
					if tc.preflightMethod != "" && (!strings.Contains(vary, "access-control-request-method") || !strings.Contains(vary, "access-control-request-headers")) {
						t.Fatal(vary)
					}
					if (resp.Header.Get("Access-Control-Allow-Origin") != "") != tc.allow {
						t.Fatal(resp.Header)
					}
					if tc.name == "allowed" && (resp.Header.Get("Access-Control-Max-Age") != "60" || resp.Header.Get("Access-Control-Allow-Credentials") != "true") {
						t.Fatal(resp.Header)
					}
					if tc.name == "normal allowed" && resp.Header.Get("Access-Control-Expose-Headers") != "X-Response-ID" {
						t.Fatal(resp.Header)
					}
				})
			}
			req, err := http.NewRequest("GET", base+"/failure", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Origin", "https://app.example")
			response, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 500 || response.Header.Get("Access-Control-Allow-Origin") != "https://app.example" {
				t.Fatalf("error response: %d %v", response.StatusCode, response.Header)
			}
			if probe.calls.Load() != 4 {
				t.Fatalf("unexpected handler calls: %d", probe.calls.Load())
			}
		})
	}
}

func TestCORSRoutePolicyRequiresExplicitOptions(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		t.Run(engine, func(t *testing.T) {
			probe := &corsProbe{}
			policy := cors.New(cors.Config{AllowOrigins: []string{"https://app.example"}, AllowMethods: []string{"GET"}})
			app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{})))
			app.Constructor(func() *corsProbe { return probe })
			app.Route("GET", "/implicit", (*corsProbe).Resource, route.WithInterceptors(policy))
			app.Route("GET", "/explicit", (*corsProbe).Resource, route.WithInterceptors(policy))
			app.Route("OPTIONS", "/explicit", (*corsProbe).Options, route.WithInterceptors(policy))
			base, client := managedServer(t, prepare(t, app, boot.HTTPOptions{}))
			for _, tc := range []struct {
				path   string
				status int
			}{{"/implicit", 404}, {"/explicit", 204}} {
				req, _ := http.NewRequest("OPTIONS", base+tc.path, nil)
				req.Header.Set("Origin", "https://app.example")
				req.Header.Set("Access-Control-Request-Method", "GET")
				resp, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
				if resp.StatusCode != tc.status {
					t.Fatalf("%s: %d", tc.path, resp.StatusCode)
				}
			}
			if probe.calls.Load() != 0 {
				t.Fatal("preflight executed an application handler")
			}
		})
	}
}
