// Run from cmd/reference with: go run ./websocket
package main

import (
	"context"
	"encoding/json"
	"github.com/NARUBROWN/spine"
	echo "github.com/NARUBROWN/spine-echo-platform"
	gorilla "github.com/NARUBROWN/spine-gorilla-platform"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
	sc "github.com/NARUBROWN/spine/pkg/spine"
	"github.com/NARUBROWN/spine/pkg/ws"
	"log"
)

type ChatMessage struct {
	Text string `json:"text"`
}
type ChatService struct{ sender ws.Sender }

func (*ChatService) Constructor(sender ws.Sender) *ChatService { return &ChatService{sender} }
func (s *ChatService) Send(ctx context.Context, id string, message ChatMessage) error {
	data, err := json.Marshal(struct {
		Type         string `json:"type"`
		ConnectionID string `json:"connectionId"`
		Text         string `json:"text"`
	}{"chat.message", id, message.Text})
	if err != nil {
		return err
	}
	return s.sender.Send(ctx, id, ws.TextMessage, data)
}

type ChatController struct{ service *ChatService }

func (*ChatController) Constructor(service *ChatService) *ChatController {
	return &ChatController{service}
}
func (c *ChatController) RegisterRoutes(r *api.Registry) {

	r.WS("/ws", func(ctx sc.Ctx, id ws.ConnectionID, message input.Message[ChatMessage]) error {
		return c.service.Send(ctx.ExecutionContext().Context(), id.Value, message.Value)
	})

	r.POST("/push/:id", func(ctx sc.Ctx, id input.Path[string], body input.Body[ChatMessage]) (httpx.Response[string], error) {
		err := c.service.Send(ctx.ExecutionContext().Context(), *id, *body)
		return httpx.Response[string]{Body: "sent"}, err
	})

}
func BuildApplication() (spine.App, boot.Options, error) {
	app := spine.New(echo.Option(), gorilla.Option())
	app.Constructor(constructor.Service(&ChatService{}), constructor.Controller(&ChatController{}))
	return app, boot.Options{Address: ":8080", HTTP: &boot.HTTPOptions{}, EnableGracefulShutdown: true}, nil
}

func main() {
	app, options, err := BuildApplication()
	if err != nil {
		log.Fatal(err)
	}
	if err := app.Run(options); err != nil {
		log.Fatal(err)
	}
}
