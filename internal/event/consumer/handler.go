package consumer

import (
	"context"
	"fmt"
	"reflect"

	validation "github.com/NARUBROWN/spine/contract/validator"
	eventresolver "github.com/NARUBROWN/spine/internal/event/consumer/resolver"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/spine"
)

type messageContext struct {
	context.Context
	values execution.Store
}

func (c *messageContext) Set(key string, value any)  { c.values.Set(key, value) }
func (c *messageContext) Get(key string) (any, bool) { return c.values.Get(key) }

type contextResolver struct{}

func (contextResolver) Supports(meta resolver.ParameterMeta) bool {
	return meta.Type == reflect.TypeFor[spine.EventCtx]()
}

func (contextResolver) Resolve(ctx spine.EventCtx, _ resolver.ParameterMeta, _ *eventresolver.Input) (any, error) {
	return ctx, nil
}

func ArgumentResolvers(validator validation.Validator) []eventresolver.ArgumentResolver {
	return []eventresolver.ArgumentResolver{
		contextResolver{},
		&eventresolver.EventNameResolver{},
		&eventresolver.PayloadResolver{},
		&eventresolver.DTOResolver{Validator: validator},
	}
}

func (r *Runtime) executeMessage(parent context.Context, reg Registration, msg *Message) (err error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	messageCtx := &messageContext{Context: ctx}
	defer messageCtx.values.ReleaseStore()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = execution.PanicError(recovered)
		}
	}()

	input := &eventresolver.Input{Name: reg.Topic, Payload: msg.Payload}
	parameters := resolver.HandlerParameters(reg.Meta, nil)
	args := make([]any, 0, len(parameters))
	for _, parameter := range parameters {
		value, err := r.resolveArgument(messageCtx, parameter, input)
		if err != nil {
			return err
		}
		args = append(args, value)
	}
	results, err := r.invoker.InvokeHandler(reg.Meta, args)
	if err != nil {
		return err
	}
	if err := invoker.ReturnedError(results); err != nil {
		return err
	}
	for _, result := range results {
		if !invoker.IsNilResult(result) {
			return fmt.Errorf("unsupported event handler return value (%T)", result)
		}
	}
	return nil
}

func (r *Runtime) resolveArgument(ctx spine.EventCtx, parameter resolver.ParameterMeta, input *eventresolver.Input) (any, error) {
	for _, candidate := range r.argumentResolvers {
		if candidate.Supports(parameter) {
			return candidate.Resolve(ctx, parameter, input)
		}
	}
	return nil, fmt.Errorf("no ArgumentResolver is registered for parameter %d (%s)", parameter.Index, parameter.Type)
}
