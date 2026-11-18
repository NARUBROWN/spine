package main

import (
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
	"github.com/NARUBROWN/spine/pkg/spine"
	"github.com/NARUBROWN/spine/pkg/ws"
)

type eventPublishingConfig struct{ enabled bool }
type UserController struct {
	publishEnabled bool
	publisher      publish.Publisher
}

func (*UserController) Constructor(config eventPublishingConfig, publisher publish.Publisher) *UserController {
	return &UserController{publishEnabled: config.enabled, publisher: publisher}
}

func (c *UserController) RegisterRoutes(r *api.Registry) {

	var logging *LoggingInterceptor

	r.GET("/users/:id", func(ctx spine.Ctx, path input.Path[UserPath]) (httpx.Response[User], error) {
		value, ok := ctx.ExecutionContext().Get("test")
		if !ok {
			return httpx.Response[User]{}, httperr.BadRequest("context does not contain content")
		}
		log.Printf("request value: %v, user: %d", value, path.ID)
		return httpx.Response[User]{}, httperr.NotFound("user not found")
	}, api.Interceptors(logging))

	r.GET("/users", func(ctx spine.Ctx, query input.Query[UserQuery]) httpx.Response[string] {
		return httpx.Response[string]{Body: "OK"}
	})

	r.POST("/upload", func(ctx spine.Ctx, form input.Body[CreatePostForm], query input.Query[UploadQuery]) (httpx.Response[string], error) {
		if form == nil {
			return httpx.Response[string]{}, httperr.BadRequest("form is required")
		}
		page, size := 1, 20
		if query.Page != nil {
			page = *query.Page
		}
		if query.Size != nil {
			size = *query.Size
		}
		log.Printf("form title=%q page=%d size=%d files=%d", form.Title, page, size, len(form.Files))
		for i, file := range form.Files {
			fmt.Printf("file #%d name=%s size=%d contentType=%s\n", i, file.Filename, file.Size, file.Header.Get("Content-Type"))
		}
		return httpx.Response[string]{Body: "OK"}, nil
	})

	r.POST("/orders/:orderId", func(ctx spine.Ctx, path input.Path[OrderPath]) (httpx.Response[string], error) {
		if !c.publishEnabled {
			return httpx.Response[string]{}, &httperr.HTTPError{Status: http.StatusServiceUnavailable, Message: "Event publishing is not configured."}
		}
		at := time.Now()
		if err := c.publisher.Event(ctx.ExecutionContext().Context(), "order.created", publish.Fields{"order_id": path.OrderID, "at": at}, publish.WithOccurredAt(at)); err != nil {
			return httpx.Response[string]{}, err
		}
		return httpx.Response[string]{Body: "OK"}, nil
	})

	r.POST("/stocks/:stockId", func(ctx spine.Ctx, path input.Path[StockPath]) (httpx.Response[string], error) {
		if !c.publishEnabled {
			return httpx.Response[string]{}, &httperr.HTTPError{Status: http.StatusServiceUnavailable, Message: "Event publishing is not configured."}
		}
		at := time.Now()
		if err := c.publisher.Event(ctx.ExecutionContext().Context(), "stock.created", publish.Fields{"stock_id": path.StockID, "at": at}, publish.WithOccurredAt(at)); err != nil {
			return httpx.Response[string]{}, err
		}
		return httpx.Response[string]{Body: "OK"}, nil
	})

}

type CommonController struct{}

func (*CommonController) Constructor() *CommonController { return &CommonController{} }

func (c *CommonController) RegisterRoutes(r *api.Registry) {

	r.GET("/headers", func(ctx spine.Ctx, headers input.Header[RequestHeaders]) httpx.Response[Headers] {
		return httpx.Response[Headers]{Body: Headers{UserAgent: headers.UserAgent, ContentType: headers.ContentType}}
	})

}

type ChatController struct{ sender ws.Sender }

func (*ChatController) Constructor(sender ws.Sender) *ChatController {
	return &ChatController{sender: sender}
}

func (c *ChatController) RegisterRoutes(r *api.Registry) {

	r.WS("/ws/chat", func(ctx spine.WebSocketCtx, id ws.ConnectionID, message input.Message[ChatMessage]) error {
		return c.sender.Send(ctx, id.Value, ws.TextMessage, []byte(message.Value.Message))
	})

}

type UserPath struct {
	ID int64 `path:"id"`
}
type OrderPath struct {
	OrderID int64 `path:"orderId"`
}
type StockPath struct {
	StockID int64 `path:"stockId"`
}
type User struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
type UserQuery struct {
	Name string `query:"name"`
}
type CreatePostForm struct {
	Title   string                  `form:"title"`
	Content string                  `form:"content"`
	Files   []*multipart.FileHeader `form:"files"`
}
type UploadQuery struct {
	Page *int `query:"page"`
	Size *int `query:"size"`
}
type Headers struct {
	UserAgent   string `json:"user_agent,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}
type RequestHeaders struct {
	UserAgent   string `header:"User-Agent"`
	ContentType string `header:"Content-Type"`
}
type ChatMessage struct {
	Message string `json:"message"`
}
