package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/event/hook"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/handler"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

func TestHTTPErrorClassificationIsIndependentOfOrigin(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"invalid_informational", &httperr.HTTPError{Status: 100, Message: "private failure"}, 500},
		{"invalid_success", &httperr.HTTPError{Status: 200, Message: "private failure"}, 500},
		{"invalid_redirect", &httperr.HTTPError{Status: 302, Message: "private failure"}, 500},
		{"below_error_range", &httperr.HTTPError{Status: 399, Message: "private failure"}, 500},
		{"first_error_status", &httperr.HTTPError{Status: 400, Message: "explicit"}, 400},
		{"last_error_status", &httperr.HTTPError{Status: 599, Message: "explicit"}, 599},
		{"wrapped_forbidden", fmt.Errorf("wrapped: %w", &httperr.HTTPError{Status: 403, Message: "explicit"}), 403},
		{"explicit_over_implicit", &httperr.HTTPError{Status: 422, Message: "explicit", Cause: &http.MaxBytesError{Limit: 42}}, 422},
		{"size_limit", &http.MaxBytesError{Limit: 42}, 413},
		{"publisher", hook.ErrPublisherUnavailable, 503},
		{"validation", validation.NewError(validation.FieldError{Field: "/name", Rule: "required"}), 400},
		{"generic", errors.New("private failure"), 500},
		{"invalid_status", &httperr.HTTPError{Status: 600, Message: "private failure"}, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reference json.RawMessage
			for _, origin := range []string{"handler", "global_pre", "route_pre", "global_before", "route_before"} {
				t.Run(origin, func(t *testing.T) {
					meta := execution.HandlerMeta{Callback: reflect.ValueOf(func() error {
						if origin == "handler" {
							return tc.err
						}
						return nil
					})}
					events := []string{}
					interceptor := &testInterceptor{name: "source", events: &events}
					switch origin {
					case "global_pre", "route_pre":
						interceptor.preErr = tc.err
					case "global_before", "route_before":
						interceptor.beforeErr = tc.err
					}
					if origin == "route_pre" || origin == "route_before" {
						meta.Interceptors = []core.Interceptor{interceptor}
					}
					p := NewPipeline(&testRouter{meta: meta}, invoker.NewInvoker(container.New()))
					p.AddReturnValueHandler(&handler.ErrorReturnHandler{})
					if origin == "global_pre" || origin == "global_before" {
						p.AddInterceptor(interceptor)
					}
					ctx := newTestExecutionContext()
					writer := &testResponseWriter{}
					ctx.SetResponseWriter(writer)
					err := p.Execute(ctx)
					if !errors.Is(err, tc.err) || writer.status != tc.status || writer.writes != 1 {
						t.Fatalf("오류 발생 위치에 따라 응답이 달라졌습니다: status=%d writes=%d err=%v", writer.status, writer.writes, err)
					}
					body := writer.body.(json.RawMessage)
					var httpErr *httperr.HTTPError
					if errors.As(tc.err, &httpErr) && (httpErr.Status < 400 || httpErr.Status > 599) {
						if !strings.Contains(err.Error(), fmt.Sprintf("invalid HTTP error status %d", httpErr.Status)) ||
							string(body) != `{"message":"Internal server error"}` {
							t.Fatalf("잘못된 오류 상태를 보고하고 공개 메시지를 숨겨야 합니다: body=%s err=%v", body, err)
						}
					}
					if origin == "handler" {
						reference = body
					} else if string(body) != string(reference) {
						t.Fatalf("같은 오류의 본문이 다릅니다: got=%s want=%s", body, reference)
					}
				})
			}
		})
	}
}

func TestBeforeResponseErrorOverridesHandlerAndPendingResponse(t *testing.T) {
	for _, scenario := range []string{"handler_error", "success", "direct_write", "direct_write_then_error"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := newTestExecutionContext()
			writer := &testResponseWriter{}
			ctx.SetResponseWriter(writer)
			original := httperr.BadRequest("handler error")
			callback := func() (httpx.Response[string], error) {
				if scenario == "direct_write" || scenario == "direct_write_then_error" {
					if err := ctx.ResponseWriter().WriteString(201, "pending"); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "handler_error" || scenario == "direct_write_then_error" {
					return httpx.Response[string]{}, original
				}
				return httpx.Response[string]{Body: "success"}, nil
			}
			meta := execution.HandlerMeta{Callback: reflect.ValueOf(callback)}
			p := NewPipeline(&testRouter{meta: meta}, invoker.NewInvoker(container.New()))
			p.AddReturnValueHandler(&handler.StringReturnHandler{}, &handler.ErrorReturnHandler{})
			events := []string{}
			override := httperr.Unauthorized("interceptor error")
			var completed error
			p.AddInterceptor(&testInterceptor{name: "override", events: &events, beforeErr: override, after: func(_ core.HandlerMeta, err error) { completed = err }})
			err := p.Execute(ctx)
			if !errors.Is(err, override) || !errors.Is(completed, override) || writer.status != 401 || writer.writes != 1 ||
				string(writer.body.(json.RawMessage)) != `{"message":"interceptor error"}` {
				t.Fatalf("인터셉터 오류를 최종 응답에 우선해야 합니다: status=%d writes=%d body=%s err=%v", writer.status, writer.writes, writer.body, err)
			}
			if scenario == "handler_error" || scenario == "direct_write_then_error" {
				if !errors.Is(err, original) || !errors.Is(completed, original) {
					t.Fatalf("핸들러의 원래 오류도 보존해야 합니다: execution=%v completion=%v", err, completed)
				}
			}
		})
	}
}

func TestLastBeforeResponseErrorDeterminesHTTPResponse(t *testing.T) {
	original := httperr.BadRequest("handler error")
	routeErr := httperr.Unauthorized("route error")
	globalErr := &httperr.HTTPError{Status: 429, Message: "global error"}
	events := []string{}
	meta := execution.HandlerMeta{
		Callback:     reflect.ValueOf(func() error { return original }),
		Interceptors: []core.Interceptor{&testInterceptor{name: "route", events: &events, beforeErr: routeErr}},
	}
	p := NewPipeline(&testRouter{meta: meta}, invoker.NewInvoker(container.New()))
	p.AddReturnValueHandler(&handler.ErrorReturnHandler{})
	p.AddInterceptor(&testInterceptor{name: "global", events: &events, beforeErr: globalErr})
	ctx := newTestExecutionContext()
	writer := &testResponseWriter{}
	ctx.SetResponseWriter(writer)
	err := p.Execute(ctx)
	if !errors.Is(err, original) || !errors.Is(err, routeErr) || !errors.Is(err, globalErr) || writer.status != 429 {
		t.Fatalf("마지막 인터셉터 오류를 응답에 적용하고 모든 원인을 보존해야 합니다: status=%d err=%v", writer.status, err)
	}
}

func TestInvalidInterceptorHTTPErrorDoesNotRestoreHandlerResponse(t *testing.T) {
	for _, status := range []int{200, 302} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			original := httperr.BadRequest("handler error")
			override := &httperr.HTTPError{Status: status, Message: "invalid interceptor error"}
			meta := execution.HandlerMeta{Callback: reflect.ValueOf(func() error { return original })}
			p := NewPipeline(&testRouter{meta: meta}, invoker.NewInvoker(container.New()))
			p.AddReturnValueHandler(&handler.ErrorReturnHandler{})
			events := []string{}
			p.AddInterceptor(&testInterceptor{name: "override", events: &events, beforeErr: override})
			ctx := newTestExecutionContext()
			writer := &testResponseWriter{}
			ctx.SetResponseWriter(writer)
			err := p.Execute(ctx)
			if !errors.Is(err, original) || !errors.Is(err, override) || writer.status != 500 || writer.writes != 1 ||
				!strings.Contains(err.Error(), fmt.Sprintf("invalid HTTP error status %d", status)) ||
				string(writer.body.(json.RawMessage)) != `{"message":"Internal server error"}` {
				t.Fatalf("잘못된 인터셉터 오류는 500으로 처리하고 원래 오류도 보존해야 합니다: status=%d err=%v", writer.status, err)
			}
		})
	}
}
