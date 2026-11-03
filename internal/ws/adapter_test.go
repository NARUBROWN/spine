package ws

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	wsadapter "github.com/NARUBROWN/spine/contract/websocket"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type upgradeFunc func(http.ResponseWriter, *http.Request) (wsadapter.Connection, error)

func (f upgradeFunc) Upgrade(w http.ResponseWriter, r *http.Request) (wsadapter.Connection, error) {
	return f(w, r)
}

type closeOnlyConnection struct{ closes int }

func (*closeOnlyConnection) ReadMessage() (int, []byte, error) {
	panic("invalid upgrade must not read")
}
func (*closeOnlyConnection) WriteMessage(int, []byte, time.Time) error {
	panic("invalid upgrade must not write")
}
func (*closeOnlyConnection) Ping(time.Time) error { panic("invalid upgrade must not ping") }
func (*closeOnlyConnection) WriteClose(int, string, time.Time) error {
	panic("invalid upgrade must not write close")
}
func (c *closeOnlyConnection) Close() error { c.closes++; return nil }

func TestRuntimePoliciesRunBeforeSelectedAdapter(t *testing.T) {
	for _, policy := range []string{"origin", "authentication", "capacity"} {
		t.Run(policy, func(t *testing.T) {
			runtime, registration := newTestRuntime(t, &cancellationController{}, (*cancellationController).Wait, boot.WebSocketOptions{MaxConnections: 1})
			defer runtime.Stop()
			runtime.adapter = upgradeFunc(func(http.ResponseWriter, *http.Request) (wsadapter.Connection, error) {
				t.Fatal("커널 정책을 거부한 요청은 플랫폼에 전달하면 안 됩니다")
				return nil, nil
			})
			request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
			want := http.StatusForbidden
			switch policy {
			case "origin":
				request.Header.Set("Origin", "https://foreign.test")
			case "authentication":
				runtime.handshakeInterceptors = []core.WebSocketHandshakeInterceptor{&rejectingHandshakeInterceptor{}}
				want = http.StatusUnauthorized
			case "capacity":
				runtime.reserveConnectionSlot()
				defer runtime.releaseConnectionSlot()
				want = http.StatusServiceUnavailable
			}
			recorder := httptest.NewRecorder()
			runtime.HandleConn(recorder, request, registration)
			if recorder.Code != want {
				t.Fatalf("정책 거부 상태=%d, 기대값=%d", recorder.Code, want)
			}
		})
	}
}

func TestRuntimeInvalidUpgradeReleasesSlotAndClosesReturnedConnection(t *testing.T) {
	var typedNil *closeOnlyConnection
	partial := &closeOnlyConnection{}
	for _, tc := range []struct {
		conn wsadapter.Connection
		err  error
	}{{nil, nil}, {typedNil, nil}, {partial, errors.New("partial upgrade failure")}} {
		runtime, registration := newTestRuntime(t, &cancellationController{}, (*cancellationController).Wait, boot.WebSocketOptions{MaxConnections: 1})
		runtime.adapter = upgradeFunc(func(w http.ResponseWriter, _ *http.Request) (wsadapter.Connection, error) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("platform response"))
			return tc.conn, tc.err
		})
		recorder := httptest.NewRecorder()
		runtime.HandleConn(recorder, httptest.NewRequest(http.MethodGet, "http://example.test/", nil), registration)
		if runtime.connectionSlots != 0 || recorder.Code != http.StatusBadRequest || recorder.Body.String() != "platform response" {
			t.Fatalf("슬롯을 해제하고 플랫폼 응답을 보존해야 합니다: slots=%d status=%d body=%s", runtime.connectionSlots, recorder.Code, recorder.Body.String())
		}
		runtime.Stop()
	}
	if partial.closes != 1 {
		t.Fatalf("실패와 함께 반환된 연결을 닫아야 합니다: %d", partial.closes)
	}
}
