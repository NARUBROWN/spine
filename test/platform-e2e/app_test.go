package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	echo "github.com/NARUBROWN/spine-echo-platform"
	gorillaplatform "github.com/NARUBROWN/spine-gorilla-platform"
	platform "github.com/NARUBROWN/spine-validator-playground"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
	ws "github.com/NARUBROWN/spine/pkg/ws"
	"github.com/gorilla/websocket"
)

type inputDTO struct {
	Email string `json:"email" form:"email" validate:"required,email"`
	Age   int    `json:"age" form:"age" validate:"gte=18"`
}

type formDTO struct {
	Name string `form:"display_name" validate:"required"`
}
type panicDTO struct {
	Value string `json:"value" validate:"undefined_rule"`
}
type controller struct{ calls atomic.Int32 }

func (c *controller) JSON(dto *inputDTO) httpx.Response[string] {
	c.calls.Add(1)
	return httpx.Response[string]{Body: dto.Email}
}
func (c *controller) Form(dto *formDTO) httpx.Response[string] {
	c.calls.Add(1)
	return httpx.Response[string]{Body: dto.Name}
}
func (c *controller) Panic(*panicDTO) httpx.Response[string] {
	c.calls.Add(1)
	return httpx.Response[string]{Body: "unreachable"}
}
func (c *controller) WS(ctx context.Context, dto inputDTO) error {
	c.calls.Add(1)
	data, err := json.Marshal(dto)
	if err != nil {
		return err
	}
	return ws.Send(ctx, ws.TextMessage, data)
}

func server(t *testing.T, c *controller, options ...platform.Option) *httptest.Server {
	t.Helper()
	v, err := platform.New(options...)
	if err != nil {
		t.Fatal(err)
	}
	app := spine.New(echo.Option(), gorillaplatform.Option(), spine.WithValidator(v))
	app.Constructor(func() *controller { return c })
	app.Route(http.MethodPost, "/json", (*controller).JSON)
	app.Route(http.MethodPost, "/form", (*controller).Form)
	app.Route(http.MethodPost, "/panic", (*controller).Panic)
	if err := app.WebSocket().Register("/ws", (*controller).WS); err != nil {
		t.Fatal(err)
	}
	if err := app.Validate(boot.Options{HTTP: &boot.HTTPOptions{}}); err != nil {
		t.Fatal(err)
	}
	prepared, err := app.Prepare(context.Background(), boot.Options{HTTP: &boot.HTTPOptions{}})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(prepared.Handler())
	srv.Client().Timeout = 5 * time.Second
	t.Cleanup(func() {
		srv.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := prepared.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return srv
}

func TestE2EHTTP(t *testing.T) {
	c := &controller{}
	srv := server(t, c)
	for _, tt := range []struct {
		name, path, body string
		status           int
		fields           bool
		calls            int32
	}{
		{"valid", "/json", `{"email":"kim@example.com","age":20}`, 200, false, 1},
		{"missing", "/json", `{}`, 400, true, 0},
		{"invalid email", "/json", `{"email":"private-invalid","age":20}`, 400, true, 0},
		{"malformed", "/json", `{`, 400, false, 0},
		{"type mismatch", "/json", `{"email":123}`, 400, false, 0},
		{"invalid rule", "/panic", `{"value":"private-input"}`, 500, false, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := c.calls.Load()
			resp, err := srv.Client().Post(srv.URL+tt.path, "application/json", strings.NewReader(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tt.status || c.calls.Load()-before != tt.calls {
				t.Fatalf("status=%d calls=%d body=%s", resp.StatusCode, c.calls.Load()-before, data)
			}
			if tt.status >= 400 {
				var body struct {
					Fields []validation.FieldError `json:"fields"`
				}
				if err := json.Unmarshal(data, &body); err != nil {
					t.Fatal(err)
				}
				if (len(body.Fields) > 0) != tt.fields {
					t.Fatalf("필드 오류 결과: %s", data)
				}
				if strings.Contains(string(data), "private-") {
					t.Fatalf("거부된 입력이 노출됐습니다: %s", data)
				}
			}
		})
	}
	var workers sync.WaitGroup
	for range 12 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			resp, err := srv.Client().Post(srv.URL+"/json", "application/json", strings.NewReader(`{"email":"kim@example.com","age":20}`))
			if err != nil {
				t.Error(err)
			} else {
				resp.Body.Close()
				if resp.StatusCode != 200 {
					t.Error(resp.StatusCode)
				}
			}
		}()
	}
	workers.Wait()
	if c.calls.Load() != 13 {
		t.Fatalf("병렬 요청 결과 calls=%d", c.calls.Load())
	}
}

func TestE2EForm(t *testing.T) {
	c := &controller{}
	srv := server(t, c, platform.WithFieldTags("form"))
	for _, body := range []string{"display_name=Kim", "display_name="} {
		resp, err := srv.Client().Post(srv.URL+"/form", "application/x-www-form-urlencoded", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(body, "Kim") {
			if resp.StatusCode != 200 {
				t.Fatalf("%d %s", resp.StatusCode, data)
			}
		} else {
			var parsed struct {
				Fields []validation.FieldError `json:"fields"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != 400 || len(parsed.Fields) != 1 || parsed.Fields[0].Field != "/display_name" {
				t.Fatalf("%d %s", resp.StatusCode, data)
			}
		}
	}
	if c.calls.Load() != 1 {
		t.Fatal(c.calls.Load())
	}
}

func TestE2EWebSocket(t *testing.T) {
	c := &controller{}
	srv := server(t, c)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	valid := inputDTO{Email: "kim@example.com", Age: 20}
	if err := conn.WriteJSON(valid); err != nil {
		t.Fatal(err)
	}
	var response inputDTO
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	if response != valid {
		t.Fatal(response)
	}
	if err := conn.WriteJSON(inputDTO{}); err != nil {
		t.Fatal(err)
	}
	_, _, err = conn.ReadMessage()
	if !websocket.IsCloseError(err, websocket.ClosePolicyViolation) || c.calls.Load() != 1 {
		t.Fatalf("close=%v calls=%d", err, c.calls.Load())
	}
}
