package test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/gorilla/websocket"
)

type appValidationFunc func(context.Context, any) error

func (f appValidationFunc) Validate(ctx context.Context, value any) error { return f(ctx, value) }

type validationForm struct {
	Name string `form:"name"`
}

type validationController struct{ calls atomic.Int32 }

func (c *validationController) Body(dto *echoRequest) httpx.Response[string] {
	c.calls.Add(1)
	return httpx.Response[string]{Body: dto.Name}
}

func (c *validationController) Form(dto *validationForm) httpx.Response[string] {
	c.calls.Add(1)
	return httpx.Response[string]{Body: dto.Name}
}

func (c *validationController) Message(wsEchoRequest) { c.calls.Add(1) }

func (c *validationController) Plain() httpx.Response[string] {
	c.calls.Add(1)
	return httpx.Response[string]{Body: "no DTO"}
}

func TestAppIntegration_ValidatorHTTP(t *testing.T) {
	failure := validation.NewError(validation.FieldError{Field: "/name", Rule: "required"})
	for _, tt := range []struct {
		name, path, contentType, body          string
		validationErr                          error
		omit                                   bool
		wantStatus, wantValidated, wantHandled int
		wantFields                             bool
	}{
		{name: "JSON success", body: `{"name":"Kim"}`, wantStatus: 200, wantValidated: 1, wantHandled: 1},
		{name: "JSON rule failure", body: `{}`, validationErr: failure, wantStatus: 400, wantValidated: 1, wantFields: true},
		{name: "wrapped rule failure", body: `{}`, validationErr: fmt.Errorf("library: %w", failure), wantStatus: 400, wantValidated: 1, wantFields: true},
		{name: "malformed JSON", body: `{`, wantStatus: 400},
		{name: "wrong JSON type", body: `{"name":123}`, wantStatus: 400},
		{name: "form success", path: "/form", contentType: "application/x-www-form-urlencoded", body: "name=Kim", wantStatus: 200, wantValidated: 1, wantHandled: 1},
		{name: "form rule failure", path: "/form", contentType: "application/x-www-form-urlencoded", body: "name=", validationErr: failure, wantStatus: 400, wantValidated: 1, wantFields: true},
		{name: "processing failure", body: `{}`, validationErr: errors.New("private library detail"), wantStatus: 500, wantValidated: 1},
		{name: "empty failure", body: `{}`, validationErr: &validation.Error{}, wantStatus: 500, wantValidated: 1},
		{name: "typed nil failure", body: `{}`, validationErr: (*validation.Error)(nil), wantStatus: 500, wantValidated: 1},
		{name: "omitted validator", body: `{}`, omit: true, wantStatus: 200, wantHandled: 1},
		{name: "no DTO", path: "/plain", wantStatus: 200, wantHandled: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			controller := &validationController{}
			var validated atomic.Int32
			validator := appValidationFunc(func(ctx context.Context, dto any) error {
				validated.Add(1)
				if ctx.Value(validationContextKey{}) != "request" {
					t.Error("검증기에 요청 컨텍스트를 전달해야 합니다")
				}
				if !tt.wantFields && tt.validationErr == nil {
					name := ""
					switch dto := dto.(type) {
					case *echoRequest:
						name = dto.Name
					case *validationForm:
						name = dto.Name
					}
					if name != "Kim" {
						t.Error("바인딩된 DTO를 전달해야 합니다")
					}
				}
				return tt.validationErr
			})
			options := []spine.Option{platform.Option()}
			if !tt.omit {
				options = append(options, spine.WithValidator(validator))
			}
			app := spine.New(options...)
			app.Constructor(func() *validationController { return controller })
			app.Route(http.MethodPost, "/body", (*validationController).Body)
			app.Route(http.MethodPost, "/form", (*validationController).Form)
			app.Route(http.MethodPost, "/plain", (*validationController).Plain)
			handler := newTestHandlerFromApp(t, app)
			path, contentType := tt.path, tt.contentType
			if path == "" {
				path = "/body"
			}
			if contentType == "" {
				contentType = "application/json"
			}
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(tt.body))
			req = req.WithContext(context.WithValue(req.Context(), validationContextKey{}, "request"))
			req.Header.Set("Content-Type", contentType)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus || int(validated.Load()) != tt.wantValidated || int(controller.calls.Load()) != tt.wantHandled {
				t.Fatalf("상태=%d 검증=%d 핸들러=%d 응답=%s", rec.Code, validated.Load(), controller.calls.Load(), rec.Body.String())
			}
			if rec.Code >= 400 {
				var body struct {
					Message string                  `json:"message"`
					Fields  []validation.FieldError `json:"fields"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if (len(body.Fields) > 0) != tt.wantFields {
					t.Fatalf("처리 오류와 필드 오류를 구분해야 합니다: %s", rec.Body.String())
				}
				if tt.wantFields && (body.Fields[0].Field != "/name" || body.Fields[0].Rule != "required") {
					t.Fatalf("필드 오류를 보존해야 합니다: %+v", body.Fields)
				}
				if strings.Contains(rec.Body.String(), "private library detail") {
					t.Fatal("검증기 처리 오류를 응답으로 노출하면 안 됩니다")
				}
			}
		})
	}
}

type validationContextKey struct{}

func TestAppIntegration_ValidatorWebSocket(t *testing.T) {
	for _, processingFailure := range []bool{false, true} {
		controller := &validationController{}
		validated := make(chan wsEchoRequest, 1)
		app := spine.New(platform.Option(), spine.WithValidator(appValidationFunc(func(_ context.Context, value any) error {
			validated <- value.(wsEchoRequest)
			if processingFailure {
				return errors.New("private validation failure")
			}
			return validation.NewError(validation.FieldError{Field: "/message", Rule: "required"})
		})))
		app.Constructor(func() *validationController { return controller })
		if err := app.WebSocket().Register("/ws/validate", (*validationController).Message); err != nil {
			t.Fatal(err)
		}
		handler := newTestHandlerFromApp(t, app)
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/validate", nil)
		if err != nil {
			server.Close()
			t.Fatal(err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		t.Cleanup(func() { _ = conn.Close() })
		if err := conn.WriteJSON(wsEchoRequest{Message: "decoded"}); err != nil {
			t.Fatal(err)
		}
		_, _, err = conn.ReadMessage()
		wantClose := websocket.ClosePolicyViolation
		if processingFailure {
			wantClose = websocket.CloseInternalServerErr
		}
		if !websocket.IsCloseError(err, wantClose) || controller.calls.Load() != 0 {
			t.Fatalf("검증 실패는 핸들러 실행 없이 연결을 종료해야 합니다: %v", err)
		}
		select {
		case dto := <-validated:
			if dto.Message != "decoded" {
				t.Fatalf("메시지 DTO를 검증해야 합니다: %+v", dto)
			}
		default:
			t.Fatal("WebSocket에도 앱 검증기를 연결해야 합니다")
		}
		_ = conn.Close()
		server.Close()
	}
}
