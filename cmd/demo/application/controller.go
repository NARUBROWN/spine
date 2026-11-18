package application

import (
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
	"github.com/NARUBROWN/spine/pkg/spine"
	"github.com/NARUBROWN/spine/pkg/ws"
)

// Controller provides HTTP callbacks and a WebSocket echo endpoint.
type Controller struct{ sender ws.Sender }

func (*Controller) Constructor(sender ws.Sender) *Controller {
	return &Controller{sender: sender}
}

func (c *Controller) RegisterRoutes(r *api.Registry) {

	r.GET("/health", func(ctx spine.Ctx) httpx.Response[map[string]string] {
		return httpx.Response[map[string]string]{Body: map[string]string{"status": "ok"}}
	})

	r.GET("/users/:id", func(ctx spine.Ctx, path input.Path[UserPath]) httpx.Response[UserResponse] {
		return httpx.Response[UserResponse]{Body: UserResponse{ID: path.ID}}
	})

	r.POST("/echo", func(ctx spine.Ctx, message input.Body[EchoMessage]) httpx.Response[EchoMessage] {
		return httpx.Response[EchoMessage]{Body: *message}
	})

	r.WS("/ws/echo", func(ctx spine.WebSocketCtx, id ws.ConnectionID, message input.Message[[]byte]) error {
		return c.sender.Send(ctx, id.Value, ws.TextMessage, message.Value)
	})

}

type UserResponse struct {
	ID int64 `json:"id"`
}
type UserPath struct {
	ID int64 `path:"id"`
}
type EchoMessage struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}
