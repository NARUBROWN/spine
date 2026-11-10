package invoker

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/execution"
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

	expectedArgs := method.Type.NumIn() - 1
	if len(args) != expectedArgs {
		return nil, fmt.Errorf("method %s expects %d arguments, got %d", method.Name, expectedArgs, len(args))
	}

	values := make([]reflect.Value, len(args)+1)
	values[0], err = invocationValue(controller.Interface(), method.Type.In(0))
	if err != nil {
		return nil, fmt.Errorf("invalid receiver for method %s: %w", method.Name, err)
	}
	for idx, arg := range args {
		values[idx+1], err = invocationValue(arg, method.Type.In(idx+1))
		if err != nil {
			return nil, fmt.Errorf("invalid argument %d for method %s: %w", idx, method.Name, err)
		}
	}

	results := method.Func.Call(values)

	out := make([]any, len(results))
	for i, result := range results {
		out[i] = result.Interface()
	}

	return out, nil
}

func invocationValue(value any, target reflect.Type) (reflect.Value, error) {
	if value == nil {
		if acceptsNil(target) {
			return reflect.Zero(target), nil
		}
		return reflect.Value{}, fmt.Errorf("nil is not assignable to %v", target)
	}

	reflected := reflect.ValueOf(value)
	if isNil(reflected) && !acceptsNil(target) {
		return reflect.Value{}, fmt.Errorf("typed nil %v is not assignable to %v", reflected.Type(), target)
	}
	if reflected.Type().AssignableTo(target) {
		return reflected, nil
	}
	if reflected.Type().ConvertibleTo(target) {
		return reflected.Convert(target), nil
	}
	return reflect.Value{}, fmt.Errorf("value of type %v is not assignable to %v", reflected.Type(), target)
}

func acceptsNil(target reflect.Type) bool {
	switch target.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return true
	default:
		return false
	}
}

func isNil(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
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
	if !value.IsValid() || isNil(value) {
		return reflect.Value{}, fmt.Errorf("resolved controller %v is nil", controllerType)
	}
	i.mu.Lock()
	if cached, ok := i.cached[controllerType]; ok {
		i.mu.Unlock()
		return cached, nil
	}
	i.cached[controllerType] = value
	i.mu.Unlock()
	return value, nil
}

// InvokeHandler는 콜백에 별도 receiver나 새 컨트롤러를 추가하지 않습니다.
func (i *Invoker) InvokeHandler(meta execution.HandlerMeta, args []any) ([]any, error) {
	if !meta.Callback.IsValid() {
		return i.Invoke(meta.ControllerType, meta.Method, args)
	}
	t := meta.Callback.Type()
	if len(args) != t.NumIn() {
		return nil, fmt.Errorf("callback expects %d arguments, got %d", t.NumIn(), len(args))
	}
	values := make([]reflect.Value, len(args))
	for index, arg := range args {
		value, err := invocationValue(arg, t.In(index))
		if err != nil {
			return nil, fmt.Errorf("invalid callback argument %d: %w", index, err)
		}
		values[index] = value
	}
	results := meta.Callback.Call(values)
	out := make([]any, len(results))
	for index, value := range results {
		out[index] = value.Interface()
	}
	return out, nil
}
