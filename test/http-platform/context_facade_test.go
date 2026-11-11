package httpplatform_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
	contextapi "github.com/NARUBROWN/spine/pkg/spine"
)

type facadeState struct {
	mu        sync.Mutex
	retained  map[string]contextapi.Ctx
	responses map[string]contextapi.HTTPResponseView
	phases    map[string][]string
	done      sync.Map
}

func (s *facadeState) retain(ctx contextapi.Ctx) {
	requestID, _ := ctx.ExecutionContext().Get("requestID")
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retained[requestID.(string)] = ctx
	if response, ok := ctx.HTTPResponseContext(); ok {
		s.responses[requestID.(string)] = response
	}
}

type facadeResult struct {
	Found     bool   `json:"found"`
	Value     string `json:"value"`
	Shared    string `json:"shared"`
	RequestID string `json:"requestId"`
}

type secondFacadeController struct{ state *facadeState }

func (c *secondFacadeController) RegisterRoutes(r *api.Registry) {
	r.GET("/second", c.Read)
	r.GET("/retained/:requestID", c.ReadRetained)
}

type retainedFacadePath struct {
	RequestID string `path:"requestID"`
}

func (c *secondFacadeController) ReadRetained(ctx contextapi.Ctx, path input.Path[retainedFacadePath]) httpx.Response[facadeResult] {
	c.state.retain(ctx)
	c.state.mu.Lock()
	previous := c.state.retained[path.RequestID]
	expiredResponse := c.state.responses[path.RequestID]
	c.state.mu.Unlock()
	expiredResponse.SetHeader("X-Expired-Response", "leak")
	if err := expiredResponse.WriteString(http.StatusAccepted, "expired"); err == nil {
		panic("만료된 뷰가 응답에 쓸 수 있습니다")
	}
	previous.ExecutionContext().Set("value", "revived")
	value, found := previous.ExecutionContext().Get("value")
	_, currentFound := ctx.ExecutionContext().Get("value")
	return httpx.Response[facadeResult]{Body: facadeResult{Found: found || currentFound, Value: fmt.Sprint(value)}}
}

func (*secondFacadeController) Share(ctx contextapi.Ctx) string {
	value, _ := ctx.ExecutionContext().Get("value")
	ctx.ExecutionContext().Set("shared", value)
	return value.(string)
}

func (c *secondFacadeController) Read(ctx contextapi.Ctx) httpx.Response[facadeResult] {
	c.state.retain(ctx)
	_, found := ctx.ExecutionContext().Get("value")
	return httpx.Response[facadeResult]{Body: facadeResult{Found: found}}
}

type firstFacadePath struct {
	Mode  string `path:"mode"`
	Token string `path:"token"`
}

type firstFacadeController struct {
	state  *facadeState
	second *secondFacadeController
}

func (c *firstFacadeController) RegisterRoutes(r *api.Registry) {
	r.GET("/first/:mode/:token", c.Write)
}

func (c *firstFacadeController) Write(ctx contextapi.Ctx, path input.Path[firstFacadePath]) (httpx.Response[facadeResult], error) {
	store := ctx.ExecutionContext()
	store.Set("value", path.Token)
	shared := c.second.Share(ctx)
	c.state.retain(ctx)
	request, ok := ctx.HTTPRequestContext()
	if !ok {
		return httpx.Response[facadeResult]{}, fmt.Errorf("HTTP view is missing")
	}
	if _, ok := ctx.WebSocket(); ok {
		return httpx.Response[facadeResult]{}, fmt.Errorf("HTTP execution exposes a WebSocket view")
	}
	response, ok := ctx.HTTPResponseContext()
	if !ok {
		return httpx.Response[facadeResult]{}, fmt.Errorf("HTTP response view is missing")
	}
	response.SetHeader("X-Context-View", path.Token)
	switch path.Mode {
	case "error":
		return httpx.Response[facadeResult]{}, httperr.BadRequest("rejected")
	case "panic":
		panic("controller panic")
	}
	return httpx.Response[facadeResult]{Body: facadeResult{
		Found: true, Value: path.Token, Shared: shared,
		RequestID: request.Header("X-Request-ID"),
	}}, nil
}

type facadeObserver struct{ state *facadeState }

func (*facadeObserver) PreHandle(ctx contextapi.Ctx, _ core.HandlerMeta) error {
	request, _ := ctx.HTTPRequestContext()
	ctx.ExecutionContext().Set("requestID", request.Header("X-Request-ID"))
	return nil
}

func (o *facadeObserver) record(phase string, ctx contextapi.Ctx) {
	value, found := ctx.ExecutionContext().Get("value")
	if !found {
		return
	}
	requestID, _ := ctx.ExecutionContext().Get("requestID")
	shared, _ := ctx.ExecutionContext().Get("shared")
	o.state.mu.Lock()
	defer o.state.mu.Unlock()
	if shared != value {
		o.state.phases[requestID.(string)] = append(o.state.phases[requestID.(string)], "shared value mismatch")
	}
	o.state.phases[requestID.(string)] = append(o.state.phases[requestID.(string)], phase)
}

func (o *facadeObserver) PostHandle(ctx contextapi.Ctx, _ core.HandlerMeta) {
	o.record("post", ctx)
}
func (o *facadeObserver) BeforeResponse(ctx contextapi.Ctx, _ core.HandlerMeta, _ error) error {
	o.record("before", ctx)
	if value, ok := ctx.ExecutionContext().Get("value"); ok {
		response, _ := ctx.HTTPResponseContext()
		response.SetHeader("X-Controller-Value", value.(string))
	}
	return nil
}
func (o *facadeObserver) AfterCompletion(ctx contextapi.Ctx, _ core.HandlerMeta, _ error) {
	o.record("after", ctx)
}

func TestCommonContextSharesWithinExecutionAndIsolatesControllers(t *testing.T) {
	for _, provider := range []string{"echo", "nethttp"} {
		t.Run(provider, func(t *testing.T) {
			state := &facadeState{retained: make(map[string]contextapi.Ctx), responses: make(map[string]contextapi.HTTPResponseView), phases: make(map[string][]string)}
			app := spine.New(spine.WithHTTPAdapter(platformFactory(provider, nativeHooks{})))
			second := &secondFacadeController{state: state}
			app.Constructor(
				func() *firstFacadeController { return &firstFacadeController{state: state, second: second} },
				func() *secondFacadeController { return second },
			)
			app.Interceptor(&facadeObserver{state: state})
			prepared := prepare(t, app, boot.HTTPOptions{})
			handler, err := spine.NetHTTPHandler(prepared)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handler.ServeHTTP(w, r)
				if done, ok := state.done.Load(r.Header.Get("X-Request-ID")); ok {
					close(done.(chan struct{}))
				}
			}))
			t.Cleanup(server.Close)
			client := server.Client()
			client.Timeout = 5 * time.Second

			request := func(t *testing.T, path, requestID string, wantStatus int) facadeResult {
				t.Helper()
				done := make(chan struct{})
				state.done.Store(requestID, done)
				req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+path, nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("X-Request-ID", requestID)
				response, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil || response.StatusCode != wantStatus || response.Header.Get("X-Expired-Response") != "" {
					t.Fatalf("status=%d body=%s err=%v", response.StatusCode, body, err)
				}
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("request did not finish")
				}
				state.mu.Lock()
				retained := state.retained[requestID]
				state.mu.Unlock()
				if retained == nil {
					t.Fatal("controller did not retain its context view")
				}
				retained.ExecutionContext().Set("late", make([]byte, 1024))
				for _, key := range []string{"value", "shared", "requestID", "late"} {
					if _, ok := retained.ExecutionContext().Get(key); ok {
						t.Fatalf("retained view exposes a completed execution value: %s", key)
					}
				}
				var result facadeResult
				if wantStatus == http.StatusOK {
					if err := json.Unmarshal(body, &result); err != nil {
						t.Fatal(err)
					}
					if result.Found && (response.Header.Get("X-Context-View") != result.Value || response.Header.Get("X-Controller-Value") != result.Value) {
						t.Fatal("컨트롤러와 인터셉터의 응답 헤더가 실제 응답에 반영되지 않았습니다")
					}
				}
				return result
			}

			for _, mode := range []string{"success", "error", "panic"} {
				t.Run(mode, func(t *testing.T) {
					status := http.StatusOK
					switch mode {
					case "error":
						status = http.StatusBadRequest
					case "panic":
						status = http.StatusInternalServerError
					}
					got := request(t, "/first/"+mode+"/"+mode, mode, status)
					if status == http.StatusOK && (!got.Found || got.Value != mode || got.Shared != mode || got.RequestID != mode) {
						t.Fatalf("same-execution controller sharing failed: %+v", got)
					}
					state.mu.Lock()
					phases := append([]string(nil), state.phases[mode]...)
					state.mu.Unlock()
					want := []string{"before", "after"}
					if mode == "success" {
						want = []string{"post", "before", "after"}
					}
					if !reflect.DeepEqual(phases, want) {
						t.Fatalf("controller writes did not reach interceptor phases: %v", phases)
					}
					if got := request(t, "/second", "second-"+mode, http.StatusOK); got.Found {
						t.Fatal("a separate controller request read the previous request's value")
					}
					if got := request(t, "/retained/"+mode, "retained-"+mode, http.StatusOK); got.Found {
						t.Fatal("다른 컨트롤러가 보관된 뷰에서 종료된 값을 읽거나 되살렸습니다")
					}
				})
			}
			t.Run("concurrent", func(t *testing.T) {
				for index := range 24 {
					t.Run(fmt.Sprint(index), func(t *testing.T) {
						t.Parallel()
						token := fmt.Sprintf("parallel-%d", index)
						got := request(t, "/first/success/"+token, token, http.StatusOK)
						if got.Value != token || got.Shared != token {
							t.Fatalf("parallel execution values mixed: %+v", got)
						}
					})
				}
			})
		})
	}
}
