package invoker

import (
	"reflect"
	"sync"

	"github.com/NARUBROWN/spine/internal/container"
)

type Invoker struct {
	container *container.Container
	mu        sync.RWMutex
	cached    map[reflect.Type]reflect.Value
}

func NewInvoker(container *container.Container) *Invoker {
	return &Invoker{
		container: container,
		cached:    make(map[reflect.Type]reflect.Value),
	}
}

func (i *Invoker) Invoke(controllerType reflect.Type, method reflect.Method, args []any) ([]any, error) {
	controller, err := i.controllerValue(controllerType)
	if err != nil {
		return nil, err
	}

	values := make([]reflect.Value, len(args)+1)
	values[0] = controller
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

func (i *Invoker) controllerValue(controllerType reflect.Type) (reflect.Value, error) {
	i.mu.RLock()
	if value, ok := i.cached[controllerType]; ok {
		i.mu.RUnlock()
		return value, nil
	}
	i.mu.RUnlock()

	controller, err := i.container.Resolve(controllerType)
	if err != nil {
		return reflect.Value{}, err
	}

	value := reflect.ValueOf(controller)
	i.mu.Lock()
	if cached, ok := i.cached[controllerType]; ok {
		i.mu.Unlock()
		return cached, nil
	}
	i.cached[controllerType] = value
	i.mu.Unlock()
	return value, nil
}
