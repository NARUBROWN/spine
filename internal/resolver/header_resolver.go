package resolver

import (
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/header"
)

type HeaderResolver struct{}

func (hr *HeaderResolver) Supports(pm ParameterMeta) bool {
	return pm.Type == reflect.TypeFor[header.Values]()
}

func (hr *HeaderResolver) Resolve(ctx core.RequestContext, parameterMeta ParameterMeta) (any, error) {
	return header.NewValues(ctx.Headers()), nil
}
