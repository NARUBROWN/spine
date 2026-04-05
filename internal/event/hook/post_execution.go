package hook

import (
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/event/publish"
)

type PostExecutionHook interface {
	AfterExecution(ctx core.ExecutionContext, result []any, err error) error
}

type EventDispatchHook struct {
	Dispatcher publish.EventDispatcher
}

func (h *EventDispatchHook) AfterExecution(ctx core.ExecutionContext, results []any, err error) error {
	if err != nil {
		return nil
	}

	events := ctx.EventBus().Drain()
	if len(events) == 0 {
		return nil
	}

	return h.Dispatcher.Dispatch(ctx.Context(), events)
}
