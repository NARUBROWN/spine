package bootstrap

import (
	"context"
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/internal/configuration"
	"github.com/NARUBROWN/spine/internal/container"
)

func shutdownHookIssues(hooks []any) []configuration.ConfigIssue {
	var issues []configuration.ConfigIssue
	for index, hook := range hooks {
		value := reflect.ValueOf(hook)
		if value.IsValid() && value.Kind() == reflect.Func && !value.IsNil() {
			signature := value.Type()
			if !signature.IsVariadic() && signature.NumIn() > 0 && signature.In(0) == reflect.TypeFor[context.Context]() &&
				signature.NumOut() == 1 && signature.Out(0) == reflect.TypeFor[error]() {
				continue
			}
		}
		issues = append(issues, configuration.ConfigIssue{
			Path:    fmt.Sprintf("ShutdownHooks[%d]", index),
			Code:    "SHUTDOWN_HOOK_INVALID",
			Message: "Shutdown hook must be a non-nil, non-variadic func(context.Context, dependencies...) error.",
		})
	}
	return issues
}

func bindShutdownHooks(hooks []any, registry *container.Container) ([]func(context.Context) error, error) {
	var bound []func(context.Context) error
	for index, hook := range hooks {
		callback := reflect.ValueOf(hook)
		signature := callback.Type()
		dependencies := make([]reflect.Value, signature.NumIn()-1)
		for i := range dependencies {
			target := signature.In(i + 1)
			dependency, err := registry.Resolve(target)
			if err != nil {
				return nil, fmt.Errorf("resolve shutdown hook[%d] dependency %v: %w", index, target, err)
			}
			dependencies[i] = reflect.ValueOf(dependency)
		}
		bound = append(bound, func(ctx context.Context) error {
			args := make([]reflect.Value, 1, 1+len(dependencies))
			args[0] = reflect.ValueOf(ctx)
			args = append(args, dependencies...)
			result := callback.Call(args)[0]
			if result.IsNil() {
				return nil
			}
			return result.Interface().(error)
		})
	}
	return bound, nil
}
