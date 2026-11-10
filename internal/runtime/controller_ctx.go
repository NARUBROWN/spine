package runtime

import "github.com/NARUBROWN/spine/core"

// ControllerContext는 인자 리졸버가 사용하는 읽기 전용 컨텍스트 계약입니다.
type ControllerContext interface {
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
