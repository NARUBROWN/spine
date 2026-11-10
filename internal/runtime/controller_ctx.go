package runtime

import "github.com/NARUBROWN/spine/core"

type controllerCtxView struct {
	ec core.ExecutionContext
}

func NewControllerContext(ec core.ExecutionContext) core.ControllerContext {
	return controllerCtxView{ec: ec}
}

func (v controllerCtxView) Get(key string) (any, bool) {
	return v.ec.Get(key)
}
