package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/NARUBROWN/spine/pkg/header"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/multipart"
	"github.com/NARUBROWN/spine/pkg/path"
	"github.com/NARUBROWN/spine/pkg/query"
	"github.com/NARUBROWN/spine/pkg/spine"
)

type UserController struct{}

type CommonController struct{}

func NewUserController() *UserController {
	return &UserController{}
}

func NewCommonController() *CommonController {
	return &CommonController{}
}

type User struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (c *UserController) GetUser(ctx context.Context, userId path.Int, spineCtx spine.Ctx) (User, error) {
	v, ok := spineCtx.Get("test")
	if !ok {
		return User{}, httperr.BadRequest("컨텍스트에 내용이 없습니다.")
	}
	log.Printf("%s", v)
	return User{}, httperr.NotFound("사용자를 찾을 수 없습니다.")
}

type CreateUserRequest struct {
	Name string `json:"name"`
}

func (c *UserController) CreateUser(ctx context.Context, req *CreateUserRequest) map[string]any {
	return map[string]any{
		"name": req.Name,
	}
}

func (c *UserController) GetUserQuery(ctx context.Context, q query.Values) httpx.Response[string] {
	// user := User{
	// 	ID:   q.Int("id", 0),
	// 	Name: q.String("name"),
	// }
	return httpx.Response[string]{
		Body: "OK",
	}
}

type CreatePostForm struct {
	Title   string `form:"title"`
	Content string `form:"content"`
}

func (c *UserController) Upload(
	ctx context.Context,
	form *CreatePostForm,
	files multipart.UploadedFiles,
	page query.Pagination,
) string {

	if form == nil {
		fmt.Println("[FORM] nil")
	} else {
		fmt.Println("[FORM] Title  :", form.Title)
		fmt.Println("[FORM] Content:", form.Content)
	}

	fmt.Println("[QUERY] Page:", page.Page)
	fmt.Println("[QUERY] Size:", page.Size)

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

func (c *UserController) CreateOrder(ctx context.Context, orderId path.Int) httpx.Response[string] {
	publish.Event(ctx, OrderCreated{
		OrderID: orderId.Value,
		At:      time.Now(),
	})

	return httpx.Response[string]{
		Body: "OK",
	}
}

func (c *UserController) CreateStock(ctx context.Context, stockId path.Int) httpx.Response[string] {
	publish.Event(ctx, StockCreated{
		OrderID: stockId.Value,
		At:      time.Now(),
	})

	return httpx.Response[string]{
		Body: "OK",
	}
}

// Headers represent the response DTO in CheckHeader.
type Headers struct {
	UserAgent   string `json:"user_agent,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// CheckHeader returns UserAgent and ContentType information from the HTTP request header
func (c *CommonController) CheckHeader(headers header.Values) Headers {
	return Headers{
		UserAgent:   headers.Get("User-Agent"),
		ContentType: headers.Get("Content-Type"),
	}
}

func (c *CommonController) GetAvatar() httpx.Binary {
	data, _ := os.ReadFile("assets/spine-logo.png")
	return httpx.Binary{
		ContentType: "image/png",
		Data:        data,
	}
}
