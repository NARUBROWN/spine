package hook

import (
	"github.com/NARUBROWN/spine/internal/event/publish"
	"github.com/NARUBROWN/spine/internal/execution"
)

// PostExecutionHook은 컨트롤러와 응답 준비가 성공한 뒤에만 실행됩니다.
type PostExecutionHook interface {
	AfterExecution(ctx execution.Context) error
}

type EventDispatchHook struct {
	Dispatcher publish.EventDispatcher
}

func (h *EventDispatchHook) AfterExecution(ctx execution.Context) error {
	events := ctx.EventBus().Drain()
	if len(events) == 0 {
		return nil
	}

	return h.Dispatcher.Dispatch(ctx.Context(), events)
}
