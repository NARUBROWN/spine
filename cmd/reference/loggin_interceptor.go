package main

import (
	"log"

	"github.com/NARUBROWN/spine/core"
	contextapi "github.com/NARUBROWN/spine/pkg/spine"
)

type LoggingInterceptor struct{}

func (i *LoggingInterceptor) PreHandle(
	ctx contextapi.Ctx,
	meta core.HandlerMeta,
) error {
	httpView, _ := ctx.HTTP()
	request := httpView.Request()
	log.Printf(
		"[REQ] %s %s -> %s.%s",
		request.Method(),
		request.Path(),
		meta.ControllerType.Name(),
		meta.HandlerName(),
	)
	ctx.ExecutionContext().Set("test", "test")

	return nil
}

func (i *LoggingInterceptor) PostHandle(
	ctx contextapi.Ctx,
	meta core.HandlerMeta,
) {
	httpView, _ := ctx.HTTP()
	request := httpView.Request()
	log.Printf(
		"[RES] %s %s OK",
		request.Method(),
		request.Path(),
	)
}

func (i *LoggingInterceptor) BeforeResponse(
	ctx contextapi.Ctx,
	meta core.HandlerMeta,
	executionErr error,
) error {
	return nil
}

func (i *LoggingInterceptor) AfterCompletion(
	ctx contextapi.Ctx,
	meta core.HandlerMeta,
	err error,
) {
	if err != nil {
		httpView, _ := ctx.HTTP()
		request := httpView.Request()
		log.Printf(
			"[ERR] %s %s : %v",
			request.Method(),
			request.Path(),
			err,
		)
	}
}
