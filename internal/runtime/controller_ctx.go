package runtime

import (
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/spine"
)

type controllerCtxView struct {
	ec core.ExecutionContext
}

func NewControllerContext(ec core.ExecutionContext) spine.Ctx {
	return controllerCtxView{ec: ec}
}

func (v controllerCtxView) Get(key string) (any, bool) {
	return v.ec.Get(key)
}
