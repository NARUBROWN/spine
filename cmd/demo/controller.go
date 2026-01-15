package main

import "github.com/NARUBROWN/spine/core"

type UserController struct{}

func NewUserController() *UserController {
	return &UserController{}
}

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (C *UserController) GetUser(ctx core.Context, id int) User {
	return User{
		ID:   id,
		Name: "spine-user",
	}
}

type CreateUserRequest struct {
	Name string `json:"name"`
}

func (c *UserController) CreateUser(ctx core.Context, req CreateUserRequest) map[string]any {
	return map[string]any{
		"name": req.Name,
	}
}

type UserQuery struct {
	ID   int    `query:"id"`
	Name string `query:"name"`
}

func (c *UserController) GetUserQuery(ctx core.Context, q UserQuery) User {
	return User{
		ID:   q.ID,
		Name: q.Name,
	}
}
