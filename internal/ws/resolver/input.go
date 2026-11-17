package resolver

import (
	"context"
	"fmt"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/ws"
	"reflect"
)

// Input is borrowed from the WS runtime for one synchronous binding operation.
type Input struct {
	ConnectionID string
	Kind         int
	Payload      []byte
}
type ArgumentResolver interface {
	Supports(resolver.ParameterMeta) bool
	Resolve(context.Context, resolver.ParameterMeta, *Input) (any, error)
}
type ContextResolver struct{}

func (*ContextResolver) Supports(p resolver.ParameterMeta) bool {
	return p.Type == reflect.TypeFor[context.Context]() || p.Type == reflect.TypeFor[ws.MessageContext]()
}
func (*ContextResolver) Resolve(ctx context.Context, _ resolver.ParameterMeta, _ *Input) (any, error) {
	return ctx, nil
}
func Resolvers(v validation.Validator) []ArgumentResolver {
	return []ArgumentResolver{&ContextResolver{}, &MessageResolver{Validator: v}, &ConnectionIDResolver{}, &PayloadResolver{}, &DTOResolver{Validator: v}}
}
func Bind(ctx context.Context, parameters []resolver.ParameterMeta, source *Input, resolvers []ArgumentResolver) ([]any, error) {
	args := make([]any, 0, len(parameters))
	for _, parameter := range parameters {
		resolved := false
		for _, r := range resolvers {
			if !r.Supports(parameter) {
				continue
			}
			value, err := r.Resolve(ctx, parameter, source)
			if err != nil {
				return nil, err
			}
			args = append(args, value)
			resolved = true
			break
		}
		if !resolved {
			return nil, fmt.Errorf("no WebSocket argument resolver for parameter %d (%s)", parameter.Index, parameter.Type)
		}
	}
	return args, nil
}
