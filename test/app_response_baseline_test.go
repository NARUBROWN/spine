package test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platform "github.com/NARUBROWN/spine-echo-platform"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/labstack/echo/v5"
)

type responseBaselineEvent struct {
	phase           string
	transportCommit bool
	prepared        bool
	writtenBytes    int
	successHeader   string
}

type responseBaselineState struct {
	response       *echo.Response
	recorder       *httptest.ResponseRecorder
	events         []responseBaselineEvent
	serializeCalls int
	serializeError bool
	finalizerError error
	beforeError    error
	afterError     error
}

func (s *responseBaselineState) observe(phase string, ctx core.ExecutionContext) {
	event := responseBaselineEvent{phase: phase}
	if s.response != nil {
		event.transportCommit = s.response.Committed
	}
	if s.recorder != nil {
		event.writtenBytes = s.recorder.Body.Len()
		event.successHeader = s.recorder.Header().Get("X-Success")
	}
	if response, ok := ctx.(core.HTTPResponseContext); ok && response.ResponseWriter() != nil {
		event.prepared = response.ResponseWriter().IsCommitted()
	}

	s.events = append(s.events, event)
}

type responseBaselinePayload struct {
	state *responseBaselineState
}

func (p responseBaselinePayload) MarshalJSON() ([]byte, error) {
	p.state.serializeCalls++
	p.state.observe("serialize", nil)
	if p.state.serializeError {
		return nil, errors.New("baseline serialization failed")
	}
	return []byte(`{"result":"ok"}`), nil
}

type responseBaselineController struct {
	state *responseBaselineState
}

func (c *responseBaselineController) Reply() httpx.Response[responseBaselinePayload] {
	return httpx.Response[responseBaselinePayload]{
		Body: responseBaselinePayload{state: c.state},
		Options: httpx.ResponseOptions{
			Status:  http.StatusCreated,
			Headers: map[string]string{"X-Success": "prepared"},
			Cookies: []httpx.Cookie{
				{Name: "baseline_a", Value: "first", Path: "/"},
				{Name: "baseline_b", Value: "second", Path: "/"},
			},
		},
	}
}

func (*responseBaselineController) Binary() httpx.Binary {
	return httpx.Binary{Data: []byte{0, 1, 255}, ContentType: "application/octet-stream"}
}

func (*responseBaselineController) Redirect() httpx.Redirect {
	return httpx.Redirect{Location: "/target?next=1"}
}

func (*responseBaselineController) Text() httpx.Response[string] {
	return httpx.Response[string]{
		Body: "plain",
		Options: httpx.ResponseOptions{Status: http.StatusAccepted, Headers: map[string]string{
			"Content-Type": "text/custom",
		}},
	}
}

type responseBaselineInterceptor struct {
	state *responseBaselineState
}

func (i *responseBaselineInterceptor) PreHandle(ctx core.ExecutionContext, _ core.HandlerMeta) error {
	i.state.observe("pre", ctx)
	return nil
}

func (i *responseBaselineInterceptor) PostHandle(ctx core.ExecutionContext, _ core.HandlerMeta) {
	i.state.observe("post", ctx)
}

func (i *responseBaselineInterceptor) BeforeResponse(ctx core.ExecutionContext, _ core.HandlerMeta, err error) error {
	i.state.observe("before", ctx)
	i.state.beforeError = err
	return i.state.finalizerError
}

func (i *responseBaselineInterceptor) AfterCompletion(ctx core.ExecutionContext, _ core.HandlerMeta, err error) {
	i.state.observe("after", ctx)
	i.state.afterError = err
}

func TestAppIntegration_ResponseStagingBaseline(t *testing.T) {
	cases := []struct {
		name           string
		serializeError bool
		finalizerError error
		status         int
		message        string
		phases         string
	}{
		{name: "success", status: 201, phases: "pre,serialize,post,before,after"},
		{name: "finalization_failure", finalizerError: httperr.BadRequest("finalization rejected"), status: 400, message: "finalization rejected", phases: "pre,serialize,post,before,after"},
		{name: "serialization_failure", serializeError: true, status: 500, message: "Internal server error", phases: "pre,serialize,before,after"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := &responseBaselineState{serializeError: tc.serializeError, finalizerError: tc.finalizerError}
			app := platform.New(platform.WithEcho(func(engine *echo.Echo) {
				engine.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
					return func(ctx *echo.Context) error {
						// 요청이 끝난 뒤 재사용되는 Echo Context 자체는 보관하지 않는다.
						response, err := echo.UnwrapResponse(ctx.Response())
						if err != nil {
							return err
						}
						state.response = response
						defer func() { state.response = nil }()
						return next(ctx)
					}
				})
			}))
			app.Constructor(func() *responseBaselineController { return &responseBaselineController{state: state} })
			app.Route(http.MethodGet, "/reply", (*responseBaselineController).Reply)
			app.Interceptor(&responseBaselineInterceptor{state: state})
			handler := newHTTPBaselineHandler(t, app, boot.HTTPOptions{})
			state.recorder = httptest.NewRecorder()
			handler.ServeHTTP(state.recorder, httptest.NewRequest(http.MethodGet, "/reply", nil))
			if got := state.recorder.Code; got != tc.status {
				t.Fatalf("응답 상태=%d 기대값=%d 본문=%s", got, tc.status, state.recorder.Body.String())
			}
			if state.serializeCalls != 1 {
				t.Errorf("애플리케이션의 JSON 직렬화는 한 번이어야 합니다: %d", state.serializeCalls)
			}
			var phases []string
			for _, event := range state.events {
				phases = append(phases, event.phase)
				if event.phase == "after" {
					if !event.transportCommit || !event.prepared || event.writtenBytes == 0 {
						t.Errorf("AfterCompletion은 실제 응답 쓰기 뒤여야 합니다: %+v", event)
					}
					continue
				}
				if event.transportCommit || event.writtenBytes != 0 || event.successHeader != "" {
					t.Errorf("BeforeResponse가 끝나기 전에는 응답을 노출하면 안 됩니다: %+v", event)
				}
				if event.phase == "post" || event.phase == "before" {
					wantPrepared := !tc.serializeError
					if event.prepared != wantPrepared {
						t.Errorf("응답 준비 상태가 실제 전송 상태와 구분되어야 합니다: %+v", event)
					}
				}
			}
			if got := strings.Join(phases, ","); got != tc.phases {
				t.Errorf("실행 순서=%s 기대값=%s", got, tc.phases)
			}
			if got := state.recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("JSON Content-Type=%q", got)
			}
			if !strings.HasSuffix(state.recorder.Body.String(), "\n") {
				t.Error("기존 JSON 응답의 마지막 개행을 보존해야 합니다")
			}
			if tc.message != "" {
				var got struct{ Message string }
				if err := json.Unmarshal(state.recorder.Body.Bytes(), &got); err != nil || got.Message != tc.message {
					t.Errorf("오류 응답=%s 기대 메시지=%q 파싱 오류=%v", state.recorder.Body.String(), tc.message, err)
				}
				if state.recorder.Header().Get("X-Success") != "" || len(state.recorder.Result().Cookies()) != 0 {
					t.Error("실패한 성공 응답의 헤더와 쿠키가 남으면 안 됩니다")
				}
				if state.afterError == nil {
					t.Error("AfterCompletion이 최종 오류를 받아야 합니다")
				}
			} else {
				var got struct{ Result string }
				if err := json.Unmarshal(state.recorder.Body.Bytes(), &got); err != nil || got.Result != "ok" {
					t.Errorf("성공 응답=%s 파싱 오류=%v", state.recorder.Body.String(), err)
				}
				cookies := state.recorder.Result().Cookies()
				if state.recorder.Header().Get("X-Success") != "prepared" || len(cookies) != 2 || cookies[0].Name != "baseline_a" || cookies[0].Value != "first" || cookies[1].Name != "baseline_b" || cookies[1].Value != "second" {
					t.Errorf("성공 헤더와 복수 쿠키가 보존되어야 합니다: %v", state.recorder.Header())
				}
				if state.afterError != nil {
					t.Errorf("성공 요청의 최종 오류=%v", state.afterError)
				}
			}
			if tc.serializeError && (state.beforeError == nil || !strings.Contains(state.beforeError.Error(), "baseline serialization failed")) {
				t.Errorf("BeforeResponse가 직렬화 오류를 받아야 합니다: %v", state.beforeError)
			}
			if !tc.serializeError && state.beforeError != nil {
				t.Errorf("BeforeResponse 진입 시점의 오류=%v", state.beforeError)
			}
			if tc.finalizerError != nil && !errors.Is(state.afterError, tc.finalizerError) {
				t.Errorf("최종 오류에 BeforeResponse 오류가 보존되어야 합니다: %v", state.afterError)
			}
		})
	}
}

func TestAppIntegration_OutputFormatsBaseline(t *testing.T) {
	app := platform.New()
	app.Constructor(func() *responseBaselineController { return &responseBaselineController{} })
	app.Route(http.MethodGet, "/binary", (*responseBaselineController).Binary)
	app.Route(http.MethodGet, "/redirect", (*responseBaselineController).Redirect)
	app.Route(http.MethodGet, "/text", (*responseBaselineController).Text)
	handler := newHTTPBaselineHandler(t, app, boot.HTTPOptions{})
	cases := []struct {
		name, target, contentType, location string
		status                              int
		body                                []byte
	}{
		{name: "binary", target: "/binary", contentType: "application/octet-stream", status: 200, body: []byte{0, 1, 255}},
		{name: "redirect_has_no_generated_body", target: "/redirect", location: "/target?next=1", status: 302},
		{name: "string_preserves_explicit_content_type", target: "/text", contentType: "text/custom", status: 202, body: []byte("plain")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.target, nil))
			if recorder.Code != tc.status || !bytes.Equal(recorder.Body.Bytes(), tc.body) {
				t.Errorf("응답 상태=%d 본문=%q, 기대 상태=%d 본문=%q", recorder.Code, recorder.Body.Bytes(), tc.status, tc.body)
			}
			if got := recorder.Header().Get("Content-Type"); got != tc.contentType {
				t.Errorf("Content-Type=%q 기대값=%q", got, tc.contentType)
			}
			if got := recorder.Header().Get("Location"); got != tc.location {
				t.Errorf("Location=%q 기대값=%q", got, tc.location)
			}
		})
	}
}
