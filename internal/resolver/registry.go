package resolver

import (
	"fmt"

	"github.com/NARUBROWN/spine/core"
)

type Registry struct {
	resolvers []ArgumentResolver
}

func NewRegistry(resolvers ...ArgumentResolver) *Registry {
	return &Registry{
		resolvers: resolvers,
	}
}

// Resolve는 파라미터 타입에 맞는 Resolver를 찾아 값을 생성합니다.
func (r *Registry) Resolve(parameterMeta ParameterMeta, ctx core.Context) (any, error) {
	for _, resolver := range r.resolvers {
		if resolver.Supports(parameterMeta) {
			return resolver.Resolve(ctx, parameterMeta)
		}
	}

	return nil, fmt.Errorf(
		"해당 파라미터 타입을 처리할 ArgumentResolver가 없습니다: %v",
		parameterMeta.Type,
	)
}
