package hook

import (
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/event/extract"
	"github.com/NARUBROWN/spine/internal/event/publish"
)

type PostExecutionHook interface {
	AfterExecution(ctx core.ExecutionContext, result []any, err error)
}

type EventDispatchHook struct {
	Extractor  extract.EventExtractor
	Dispatcher publish.EventDispatcher
}

func (h *EventDispatchHook) AfterExecution(ctx core.ExecutionContext, results []any, err error) {
	if err != nil {
		return
	}

	events := ctx.EventBus().Drain()
	if len(events) == 0 {
		return
	}

	h.Dispatcher.Dispatch(ctx.Context(), events)
}
