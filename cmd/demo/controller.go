package main

import (
	"fmt"

	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/multipart"
	"github.com/NARUBROWN/spine/pkg/path"
	"github.com/NARUBROWN/spine/pkg/query"
)

type UserController struct{}

func NewUserController() *UserController {
	return &UserController{}
}

type User struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (c *UserController) GetUser(userId path.Int) (User, error) {
	return User{}, httperr.NotFound("사용자를 찾을 수 없습니다.")
}

type CreateUserRequest struct {
	Name string `json:"name"`
}

func (c *UserController) CreateUser(req *CreateUserRequest) map[string]any {
	return map[string]any{
		"name": req.Name,
	}
}

func (c *UserController) GetUserQuery(q query.Values) User {
	return User{
		ID:   q.Int("id", 0),
		Name: q.String("name"),
	}
}

type CreatePostForm struct {
	Title   string `form:"title"`
	Content string `form:"content"`
}

func (c *UserController) Upload(
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
