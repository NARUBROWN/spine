package endpoint

import (
	"errors"
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/internal/configuration"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

func TestHTTPCallbacksAllowAtMostOneResponse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler any
		path    string
	}{
		{"none", func() {}, ""},
		{"error", func() error { return nil }, ""},
		{"response", func() httpx.Response[string] { return httpx.Response[string]{} }, ""},
		{"response_error", func() (httpx.Response[string], error) { return httpx.Response[string]{}, nil }, ""},
		{"error_response", func() (error, *httpx.Binary) { return nil, nil }, ""},
		{"two_responses", func() (httpx.Response[string], httpx.Response[int]) {
			return httpx.Response[string]{}, httpx.Response[int]{}
		}, "Routes[0].Returns[1]"},
		{"response_and_binary", func() (*httpx.Response[string], *httpx.Binary, error) { return nil, nil, nil }, "Routes[0].Returns[1]"},
		{"redirect_and_response", func() (httpx.Redirect, error, httpx.Response[int]) {
			return httpx.Redirect{}, nil, httpx.Response[int]{}
		}, "Routes[0].Returns[2]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := reflect.ValueOf(tc.handler)
			issues := CallableIssues(execution.HandlerMeta{Callback: value}, "Routes[0]")
			_, err := Describe([]router.RouteSpec{{Method: "GET", Path: "/response", Handler: tc.handler, ControllerType: reflect.TypeFor[*controller]()}}, &configuration.HTTPOptions{}, nil)
			if tc.path == "" {
				if len(issues) != 0 || err != nil {
					t.Fatalf("유효한 응답 계약을 거부했습니다: issues=%v description=%v", issues, err)
				}
				return
			}
			var failure *configuration.ConfigError
			if len(issues) != 1 || issues[0].Code != "HTTP_RESPONSE_RETURNS_DUPLICATED" || issues[0].Path != tc.path ||
				!errors.As(err, &failure) || len(failure.Issues) != 1 || failure.Issues[0].Code != issues[0].Code {
				t.Fatalf("다중 응답을 준비와 명세에서 동일하게 거부해야 합니다: issues=%v description=%v", issues, err)
			}
		})
	}
}
