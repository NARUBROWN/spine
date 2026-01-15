package main

import "github.com/NARUBROWN/spine"

func main() {
	app := spine.New()

	// 생성자 등록
	app.Constructor(
		NewUserController,
	)

	// 라우트 등록
	app.Route(
		"GET",
		"/users/:id",
		(*UserController).GetUser,
	)

	app.Route(
		"POST",
		"/users",
		(*UserController).CreateUser,
	)

	app.Route(
		"GET",
		"/users",
		(*UserController).GetUserQuery,
	)

	app.Run(":8080")
}
