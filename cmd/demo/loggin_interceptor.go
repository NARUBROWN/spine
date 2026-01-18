package main

import (
	"log"

	"github.com/NARUBROWN/spine/core"
)

type LoggingInterceptor struct{}

func (i *LoggingInterceptor) PreHandle(
	ctx core.ExecutionContext,
	meta core.HandlerMeta,
) error {
	log.Printf(
		"[REQ] %s %s -> %s.%s",
		ctx.Method(),
		ctx.Path(),
		meta.ControllerType.Name(),
		meta.Method.Name,
	)
	return nil
}

func (i *LoggingInterceptor) PostHandle(
	ctx core.ExecutionContext,
	meta core.HandlerMeta,
) {
	log.Printf(
		"[RES] %s %s OK",
		ctx.Method(),
		ctx.Path(),
	)
}

func (i *LoggingInterceptor) AfterCompletion(
	ctx core.ExecutionContext,
	meta core.HandlerMeta,
	err error,
) {
	if err != nil {
		log.Printf(
			"[ERR] %s %s : %v",
			ctx.Method(),
			ctx.Path(),
			err,
		)
	}
}
