// Package application은 CLI 계약 검사와 실제 demo 실행이 공유하는 앱 구성을 제공합니다.
package application

import (
	"os"
	"time"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// BuildApplication은 리스너나 외부 브로커를 열지 않고 최소 HTTP/WS 앱을 구성합니다.
func BuildApplication() (spine.App, boot.Options, error) {
	app := spine.New(platform.Option())
	app.Constructor(NewController)
	app.Route("GET", "/health", (*Controller).Health)
	app.Route("GET", "/users/:id", (*Controller).User)
	app.Route("POST", "/echo", (*Controller).Echo)
	if err := app.WebSocket().Register("/ws/echo", (*Controller).WebSocketEcho); err != nil {
		return nil, boot.Options{}, err
	}

	address := "127.0.0.1:8080"
	if port := os.Getenv("SPINE_PORT"); port != "" {
		address = "127.0.0.1:" + port
	}
	if override := os.Getenv("SPINE_DEMO_ADDRESS"); override != "" {
		address = override
	}

	return app, boot.Options{
		Address:                address,
		EnableGracefulShutdown: true,
		ShutdownTimeout:        3 * time.Second,
		HTTP: &boot.HTTPOptions{
			MaxBodyBytes: 1024,
		},
	}, nil
}
