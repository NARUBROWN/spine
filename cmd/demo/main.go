package main

import (
	"time"

	"github.com/NARUBROWN/spine"
)

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

	// EnableGracefulShutdown & ShutdownTimeout은 선택사항입니다.
	app.Run(spine.BootOptions{
		Address:                ":8080",
		EnableGracefulShutdown: true,
		ShutdownTimeout:        10 * time.Second,
	})
}
