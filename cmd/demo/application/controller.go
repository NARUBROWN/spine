package application

import (
	"context"

	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/ws"
)

// Controller는 최소 E2E에서 사용할 HTTP와 WS 경로를 제공합니다.
type Controller struct{}

func NewController() *Controller { return &Controller{} }

func (*Controller) Health() httpx.Response[map[string]string] {
	return httpx.Response[map[string]string]{Body: map[string]string{"status": "ok"}}
}

// UserResponse는 경로 인자가 실제 컨트롤러까지 전달되는지 확인할 응답입니다.
type UserResponse struct {
	ID int64 `json:"id"`
}

type UserPath struct {
	ID int64 `path:"id"`
}

func (*Controller) User(p *UserPath) httpx.Response[UserResponse] {
	return httpx.Response[UserResponse]{Body: UserResponse{ID: p.ID}}
}

// EchoMessage는 JSON 바인딩과 응답 직렬화가 함께 사용하는 DTO입니다.
type EchoMessage struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

func (*Controller) Echo(message *EchoMessage) httpx.Response[EchoMessage] {
	return httpx.Response[EchoMessage]{Body: *message}
}

func (*Controller) WebSocketEcho(ctx context.Context, payload []byte) error {
	return ws.Send(ctx, ws.TextMessage, payload)
}
