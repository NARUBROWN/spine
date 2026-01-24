package consumer

import (
	"reflect"

	"github.com/NARUBROWN/spine/core"
)

type ParameterMeta struct {
	Index   int
	Type    reflect.Type
	PathKey string
}

type ArgumentResolver interface {
	Supports(parameterMeta ParameterMeta) bool
	Resolve(ctx core.ConsumerRequestContext, parameterMeta ParameterMeta) (any, error)
}
