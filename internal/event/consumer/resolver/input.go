package resolver

import (
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/spine"
)

type Input struct {
	Name    string
	Payload []byte
}

type ArgumentResolver interface {
	Supports(resolver.ParameterMeta) bool
	Resolve(spine.EventCtx, resolver.ParameterMeta, *Input) (any, error)
}
