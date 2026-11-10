package runtime

import (
	"context"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/event/publish"
)

// 이름 없는 인터페이스로 공개 뷰와 같은 계약을 구현합니다.
type executionView = interface {
	core.ContextCarrier
	Get(key string) (any, bool)
}

// ControllerContext는 인자 리졸버가 사용하는 읽기 전용 컨텍스트 계약입니다.
type ControllerContext interface {
	ExecutionContext() executionView
	Get(key string) (any, bool)
}

type controllerCtxView struct {
	ec core.ExecutionContext
}

func NewControllerContext(ec core.ExecutionContext) ControllerContext {
	return controllerCtxView{ec: ec}
}

func (v controllerCtxView) Get(key string) (any, bool) {
	return v.ec.Get(key)
}

func (v controllerCtxView) ExecutionContext() executionView {
	return executionCtxView{ec: v.ec}
}

type executionCtxView struct {
	ec core.ExecutionContext
}

func (v executionCtxView) Get(key string) (any, bool) {
	return v.ec.Get(key)
}

func (v executionCtxView) Context() context.Context {
	return HandlerGoContext(v.ec)
}

// HandlerGoContext는 전송 컨텍스트에 현재 실행의 이벤트 발행기를 연결합니다.
func HandlerGoContext(ec core.ExecutionContext) context.Context {
	ctx := ec.Context()
	if bus := ec.EventBus(); bus != nil {
		return publish.WithPublisher(ctx, bus)
	}
	return ctx
}
