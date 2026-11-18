// Package application shares composition between CLI checks and the demo server.
package application

import (
	"os"
	"time"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	gorillaplatform "github.com/NARUBROWN/spine-gorilla-platform"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
)

// BuildApplication assembles the HTTP and WebSocket demo without opening listeners.
func BuildApplication() (spine.App, boot.Options, error) {
	app := spine.New(gorillaplatform.Option(), platform.Option())
	app.Constructor(constructor.Controller(&Controller{}))

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
