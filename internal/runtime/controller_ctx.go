package runtime

import (
	"context"

	"github.com/NARUBROWN/spine/internal/execution"
)

// 이름 없는 인터페이스로 공개 뷰와 같은 계약을 구현합니다.
type executionView = interface {
	Context() context.Context
	Get(key string) (any, bool)
}

// ControllerContext는 인자 리졸버가 사용하는 읽기 전용 컨텍스트 계약입니다.
type ControllerContext interface {
	ExecutionContext() executionView
	Get(key string) (any, bool)
}

type controllerCtxView struct {
	ec execution.Context
}

func NewControllerContext(ec execution.Context) ControllerContext {
	return controllerCtxView{ec: ec}
}

func (v controllerCtxView) Get(key string) (any, bool) {
	return v.ec.Get(key)
}

func (v controllerCtxView) ExecutionContext() executionView {
	return executionCtxView{ec: v.ec}
}

type executionCtxView struct {
	ec execution.Context
}

func (v executionCtxView) Get(key string) (any, bool) {
	return v.ec.Get(key)
}

func (v executionCtxView) Context() context.Context {
	return execution.GoContext(v.ec)
}
