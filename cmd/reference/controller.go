package main

import httpinput "github.com/NARUBROWN/spine/pkg/input"

import (
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/spine"
	"github.com/NARUBROWN/spine/pkg/ws"
)

type UserPath struct {
	ID int64 `path:"id"`
}
type OrderPath struct {
	OrderID int64 `path:"orderId"`
}
type StockPath struct {
	StockID int64 `path:"stockId"`
}

type eventPublishingConfig struct{ enabled bool }

type UserController struct{ publishEnabled bool }

type CommonController struct{}

type ChatController struct{}

func (*UserController) Constructor(publishing eventPublishingConfig) *UserController {
	return &UserController{publishEnabled: publishing.enabled}
}

func (*CommonController) Constructor() *CommonController {
	return &CommonController{}
}

func NewChatController() *ChatController {
	return &ChatController{}
}

func (c *UserController) RegisterRoutes(r *api.Registry) {
	r.GET("/users/:id", c.GetUser, api.Interceptors(&LoggingInterceptor{}))
	r.GET("/users", c.GetUserQuery)
	r.POST("/upload", c.Upload)
	r.POST("/orders/:orderId", c.CreateOrder)
	r.POST("/stocks/:stockId", c.CreateStock)
}

func (c *CommonController) RegisterRoutes(r *api.Registry) {
	r.GET("/headers", c.CheckHeader)
	r.GET("/images/:id", c.GetAvatar)
}

type User struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (c *UserController) GetUser(ctx spine.Ctx, p httpinput.Path[UserPath]) (httpx.Response[User], error) {
	v, ok := ctx.ExecutionContext().Get("test")
	if !ok {
		return httpx.Response[User]{}, httperr.BadRequest("context does not contain content")
	}
	log.Printf("%s", v)
	return httpx.Response[User]{}, httperr.NotFound("user not found")
}

type CreateUserRequest struct {
	Name string `json:"name"`
}

func (c *UserController) CreateUser(req httpinput.Body[CreateUserRequest]) map[string]any {
	return map[string]any{
		"name": req.Name,
	}
}

type UserQuery struct {
	Name string `query:"name"`
}

func (c *UserController) GetUserQuery(q httpinput.Query[UserQuery]) httpx.Response[string] {
	return httpx.Response[string]{
		Body: "OK",
	}
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

func (c *UserController) Upload(
	form httpinput.Body[CreatePostForm],
	q httpinput.Query[UploadQuery],
) httpx.Response[string] {
	page, size := 1, 20
	if q.Page != nil {
		page = *q.Page
	}
	if q.Size != nil {
		size = *q.Size
	}

	if form == nil {
		fmt.Println("[FORM] nil")
	} else {
		fmt.Println("[FORM] Title  :", form.Title)
		fmt.Println("[FORM] Content:", form.Content)
	}

	fmt.Println("[QUERY] Page:", page)
	fmt.Println("[QUERY] Size:", size)

	fmt.Println("[FILES] Count:", len(form.Files))
	for i, f := range form.Files {
		fmt.Printf(
			"[FILES] #%d field=%s name=%s size=%d contentType=%s\n",
			i,
			"files",
			f.Filename,
			f.Size,
			f.Header.Get("Content-Type"),
		)
	}

	return httpx.Response[string]{Body: "OK"}
}

func (c *UserController) CreateOrder(ctx spine.Ctx, p httpinput.Path[OrderPath]) (httpx.Response[string], error) {
	if !c.publishEnabled {
		return httpx.Response[string]{}, &httperr.HTTPError{Status: http.StatusServiceUnavailable, Message: "Event publishing is not configured."}
	}
	publish.DomainEvents(ctx.ExecutionContext().Context(), OrderCreated{
		OrderID: p.OrderID,
		At:      time.Now(),
	})

	return httpx.Response[string]{
		Body: "OK",
	}, nil
}

func (c *UserController) CreateStock(ctx spine.Ctx, p httpinput.Path[StockPath]) (httpx.Response[string], error) {
	if !c.publishEnabled {
		return httpx.Response[string]{}, &httperr.HTTPError{Status: http.StatusServiceUnavailable, Message: "Event publishing is not configured."}
	}
	publish.DomainEvents(ctx.ExecutionContext().Context(), StockCreated{
		OrderID: p.StockID,
		At:      time.Now(),
	})

	return httpx.Response[string]{
		Body: "OK",
	}, nil
}

// Headers는 CheckHeader가 반환하는 응답 DTO입니다.
type Headers struct {
	UserAgent   string `json:"user_agent,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// CheckHeader는 HTTP 요청 헤더의 User-Agent와 Content-Type 정보를 반환합니다.
type RequestHeaders struct {
	UserAgent   string `header:"User-Agent"`
	ContentType string `header:"Content-Type"`
}

func (c *CommonController) CheckHeader(headers httpinput.Header[RequestHeaders]) httpx.Response[Headers] {
	return httpx.Response[Headers]{Body: Headers{
		UserAgent:   headers.UserAgent,
		ContentType: headers.ContentType,
	}}
}

func (c *CommonController) GetAvatar() httpx.Binary {
	data, _ := os.ReadFile("assets/spine-logo.png")
	return httpx.Binary{
		ContentType: "image/png",
		Data:        data,
	}
}

type ChatMessage struct {
	Message string `json:"message"`
}

func (c *ChatController) OnMessage(ctx spine.Ctx, connID ws.ConnectionID, msg ChatMessage) error {
	return ws.Send(ctx.ExecutionContext().Context(), ws.TextMessage, []byte(msg.Message))
}
