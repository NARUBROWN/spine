package invoker

import (
	"reflect"

	"github.com/NARUBROWN/spine/internal/container"
)

type Invoker struct {
	container *container.Container
}

func NewInvoker(container *container.Container) *Invoker {
	return &Invoker{
		container: container,
	}
}

func (i *Invoker) Invoke(controllerType reflect.Type, method reflect.Method, args []any) ([]any, error) {
	// 컨트롤러 인스턴스 Resolve
	controller, err := i.container.Resolve(controllerType)
	if err != nil {
		return nil, err
	}

	values := make([]reflect.Value, len(args)+1)
	values[0] = reflect.ValueOf(controller)
	for idx, arg := range args {
		values[idx+1] = reflect.ValueOf(arg)
	}

	results := method.Func.Call(values)

	out := make([]any, len(results))
	for i, result := range results {
		out[i] = result.Interface()
	}

	return out, nil
}
