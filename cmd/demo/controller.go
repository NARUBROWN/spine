package main

import (
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

func (c *UserController) GetUser(userId path.Int) User {
	return User{
		ID:   userId.Value,
		Name: "spine-user",
	}
}

type CreateUserRequest struct {
	Name string `json:"name"`
}

func (c *UserController) CreateUser(req CreateUserRequest) map[string]any {
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
