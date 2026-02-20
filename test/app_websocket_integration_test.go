package test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/pkg/httpx"
	pkgws "github.com/NARUBROWN/spine/pkg/ws"
	"github.com/gorilla/websocket"
)

type wsEchoRequest struct {
	Message string `json:"message"`
}

type wsEchoResponse struct {
	ConnID  string `json:"connId"`
	Payload string `json:"payload"`
	Message string `json:"message"`
}

type wsIntegrationController struct{}

func (c *wsIntegrationController) Hello() httpx.Response[string] {
	return httpx.Response[string]{Body: "ok"}
}

func (c *wsIntegrationController) Echo(ctx context.Context, connID pkgws.ConnectionID, payload []byte, dto wsEchoRequest) {
	response, _ := json.Marshal(wsEchoResponse{
		ConnID:  connID.Value,
		Payload: string(payload),
		Message: dto.Message,
	})
	_ = pkgws.Send(ctx, pkgws.TextMessage, response)
}

func setupWebSocketApp() spine.App {
	app := spine.New()
	app.Constructor(func() *wsIntegrationController { return &wsIntegrationController{} })
	app.Route("GET", "/hello", (*wsIntegrationController).Hello)
	app.WebSocket().Register("/ws/echo", (*wsIntegrationController).Echo)
	return app
}

func TestAppIntegration_WebSocketEcho(t *testing.T) {
	app := setupWebSocketApp()
	handler := newTestHandlerFromApp(t, app)

	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/echo"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("websocket 연결 실패: %v", err)
	}
	defer conn.Close()

	firstPayload := []byte(`{"message":"hello"}`)
	if err := conn.WriteMessage(websocket.TextMessage, firstPayload); err != nil {
		t.Fatalf("첫 메시지 전송 실패: %v", err)
	}

	firstConnID, first := readEchoResponse(t, conn)
	if first.Message != "hello" {
		t.Fatalf("첫 메시지 응답이 잘못되었습니다: %+v", first)
	}
	if first.Payload != string(firstPayload) {
		t.Fatalf("첫 payload 응답이 잘못되었습니다: %+v", first)
	}
	if firstConnID == "" {
		t.Fatal("connID가 비어있으면 안 됩니다")
	}

	secondPayload := []byte(`{"message":"again"}`)
	if err := conn.WriteMessage(websocket.TextMessage, secondPayload); err != nil {
		t.Fatalf("두 번째 메시지 전송 실패: %v", err)
	}

	secondConnID, second := readEchoResponse(t, conn)
	if second.Message != "again" {
		t.Fatalf("두 번째 메시지 응답이 잘못되었습니다: %+v", second)
	}
	if second.Payload != string(secondPayload) {
		t.Fatalf("두 번째 payload 응답이 잘못되었습니다: %+v", second)
	}
	if secondConnID != firstConnID {
		t.Fatalf("동일 연결에서 connID는 동일해야 합니다: first=%s, second=%s", firstConnID, secondConnID)
	}
}

func readEchoResponse(t *testing.T, conn *websocket.Conn) (string, wsEchoResponse) {
	t.Helper()

	msgType, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("응답 수신 실패: %v", err)
	}
	if msgType != websocket.TextMessage {
		t.Fatalf("응답 messageType이 잘못되었습니다: %d", msgType)
	}

	var body wsEchoResponse
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("응답 JSON 파싱 실패: %v", err)
	}
	return body.ConnID, body
}
