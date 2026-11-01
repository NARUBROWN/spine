package main

import (
	"log"

	"github.com/NARUBROWN/spine/cmd/demo/application"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	app, options, err := application.BuildApplication()
	if err != nil {
		return err
	}
	if err := app.Validate(options); err != nil {
		return err
	}
	return app.Run(options)
}
