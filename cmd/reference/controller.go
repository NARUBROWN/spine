package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/multipart"
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

type UserController struct{}

type CommonController struct{}

type ChatController struct{}

func NewUserController() *UserController {
	return &UserController{}
}

func NewCommonController() *CommonController {
	return &CommonController{}
}

func NewChatController() *ChatController {
	return &ChatController{}
}

type User struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (c *UserController) GetUser(ctx context.Context, p *UserPath, spineCtx spine.Ctx) (User, error) {
	v, ok := spineCtx.Get("test")
	if !ok {
		return User{}, httperr.BadRequest("context does not contain content")
	}
	log.Printf("%s", v)
	return User{}, httperr.NotFound("user not found")
}

type CreateUserRequest struct {
	Name string `json:"name"`
}

func (c *UserController) CreateUser(ctx context.Context, req *CreateUserRequest) map[string]any {
	return map[string]any{
		"name": req.Name,
	}
}

type UserQuery struct {
	Name string `query:"name"`
}

func (c *UserController) GetUserQuery(ctx context.Context, q *UserQuery) httpx.Response[string] {
	return httpx.Response[string]{
		Body: "OK",
	}
}

type CreatePostForm struct {
	Title   string `form:"title"`
	Content string `form:"content"`
}

type UploadQuery struct {
	Page *int `query:"page"`
	Size *int `query:"size"`
}

func (c *UserController) Upload(
	ctx context.Context,
	form *CreatePostForm,
	files multipart.UploadedFiles,
	q *UploadQuery,
) string {
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

	fmt.Println("[FILES] Count:", len(files.Files))
	for i, f := range files.Files {
		fmt.Printf(
			"[FILES] #%d field=%s name=%s size=%d contentType=%s\n",
			i,
			f.FieldName,
			f.Filename,
			f.Size,
			f.ContentType,
		)
	}

	return "OK"
}

func (c *UserController) CreateOrder(ctx context.Context, p *OrderPath) httpx.Response[string] {
	publish.Event(ctx, OrderCreated{
		OrderID: p.OrderID,
		At:      time.Now(),
	})

	return httpx.Response[string]{
		Body: "OK",
	}
}

func (c *UserController) CreateStock(ctx context.Context, p *StockPath) httpx.Response[string] {
	publish.Event(ctx, StockCreated{
		OrderID: p.StockID,
		At:      time.Now(),
	})

	return httpx.Response[string]{
		Body: "OK",
	}
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

func (c *CommonController) CheckHeader(headers *RequestHeaders) Headers {
	return Headers{
		UserAgent:   headers.UserAgent,
		ContentType: headers.ContentType,
	}
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

func (c *ChatController) OnMessage(ctx context.Context, connID ws.ConnectionID, msg ChatMessage) error {
	return ws.Send(ctx, ws.TextMessage, []byte(msg.Message))
}
