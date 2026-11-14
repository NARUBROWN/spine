package container

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

type Container struct {
	mu           sync.RWMutex
	constructors map[reflect.Type]reflect.Value
	instances    map[reflect.Type]any
	building     map[reflect.Type]*buildState
}

type buildState struct {
	done     chan struct{}
	instance any
	err      error
}

type provider struct {
	typeKey     reflect.Type
	constructor reflect.Value
}

func New() *Container {
	return &Container{
		constructors: make(map[reflect.Type]reflect.Value),
		instances:    make(map[reflect.Type]any),
		building:     make(map[reflect.Type]*buildState),
	}
}

func (c *Container) RegisterConstructor(function any) error {
	val := reflect.ValueOf(function)
	if !val.IsValid() {
		return errors.New("constructor must not be nil")
	}
	typ := val.Type()

	if typ.Kind() != reflect.Func {
		return errors.New("constructor must be a function")
	}
	if val.IsNil() {
		return errors.New("constructor must not be nil")
	}

	if !ValidConstructorResults(typ) {
		return errors.New("constructor must return a value or a value and error")
	}
	if typ.IsVariadic() {
		return errors.New("constructor must be non-variadic; use a slice parameter instead")
	}

	outType := typ.Out(0)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.constructors[outType] = val

	return nil
}

func (c *Container) Resolve(componentType reflect.Type) (any, error) {
	if componentType == nil {
		return nil, errors.New("component type must not be nil")
	}
	if err := c.validateDependencyGraph(componentType, map[reflect.Type]int{}, nil, map[reflect.Type]struct{}{}); err != nil {
		return nil, err
	}
	return c.resolve(componentType, map[reflect.Type]int{}, nil)
}

func (c *Container) validateDependencyGraph(
	componentType reflect.Type,
	stack map[reflect.Type]int,
	path []reflect.Type,
	validated map[reflect.Type]struct{},
) error {
	provider, err := c.getProvider(componentType)
	if err != nil {
		return err
	}
	providerType := provider.typeKey

	if _, ok := c.getInstance(providerType); ok {
		return nil
	}
	if _, ok := validated[providerType]; ok {
		return nil
	}
	if idx, ok := stack[providerType]; ok {
		cycle := append([]reflect.Type{}, path[idx:]...)
		cycle = append(cycle, providerType)
		return fmt.Errorf("circular dependency detected: %s", formatTypePath(cycle))
	}

	stack[providerType] = len(path)
	path = append(path, providerType)
	defer delete(stack, providerType)

	for i := 0; i < provider.constructor.Type().NumIn(); i++ {
		if err := c.validateDependencyGraph(provider.constructor.Type().In(i), stack, path, validated); err != nil {
			return err
		}
	}
	validated[providerType] = struct{}{}
	return nil
}

func (c *Container) resolve(componentType reflect.Type, stack map[reflect.Type]int, path []reflect.Type) (any, error) {
	provider, err := c.getProvider(componentType)
	if err != nil {
		return nil, err
	}
	providerType := provider.typeKey

	if idx, ok := stack[providerType]; ok {
		cycle := append([]reflect.Type{}, path[idx:]...)
		cycle = append(cycle, providerType)
		return nil, fmt.Errorf("circular dependency detected: %s", formatTypePath(cycle))
	}

	if instance, ok := c.getInstance(providerType); ok {
		return instance, nil
	}

	state, wait, ok := c.beginBuild(providerType)
	if !ok {
		return state.instance, state.err
	}
	if wait {
		select {
		case <-state.done:
			return state.instance, state.err
		}
	}
	defer c.finishBuild(providerType, state)

	stack[providerType] = len(path)
	path = append(path, providerType)
	defer delete(stack, providerType)

	numIn := provider.constructor.Type().NumIn()
	args := make([]reflect.Value, numIn)
	for i := 0; i < numIn; i++ {
		paramType := provider.constructor.Type().In(i)
		paramInstance, err := c.resolve(paramType, stack, path)
		if err != nil {
			state.err = fmt.Errorf("resolve dependency %v for %v: %w", paramType, providerType, err)
			return nil, state.err
		}
		arg, err := valueForType(paramInstance, paramType)
		if err != nil {
			state.err = fmt.Errorf("invalid dependency %d for %v: %w", i, providerType, err)
			return nil, state.err
		}
		args[i] = arg
	}

	result, err := callConstructor(provider.constructor, args)
	if err != nil {
		state.err = fmt.Errorf("construct %v: %w", providerType, err)
		return nil, state.err
	}
	if cached, existed := c.cacheInstance(providerType, result); existed {
		state.instance = cached
		return cached, nil
	}
	state.instance = result

	return result, nil
}

// ValidConstructorResults는 제공 값과 선택적 error 반환의 형식을 검사합니다.
func ValidConstructorResults(signature reflect.Type) bool {
	errorType := reflect.TypeFor[error]()
	switch signature.NumOut() {
	case 1:
		return signature.Out(0) != errorType
	case 2:
		return signature.Out(0) != errorType && signature.Out(1) == errorType
	default:
		return false
	}
}

func callConstructor(constructor reflect.Value, args []reflect.Value) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if recoveredErr, ok := recovered.(error); ok {
				err = fmt.Errorf("panic while executing constructor: %w", recoveredErr)
				return
			}
			err = fmt.Errorf("panic while executing constructor: %v", recovered)
		}
	}()

	results := constructor.Call(args)
	if len(results) == 2 && !results[1].IsNil() {
		return nil, results[1].Interface().(error)
	}
	value := results[0]
	if isNilValue(value) {
		return nil, fmt.Errorf("constructor for %v returned nil", constructor.Type().Out(0))
	}
	return value.Interface(), nil
}

func valueForType(value any, target reflect.Type) (reflect.Value, error) {
	if value == nil {
		return reflect.Value{}, fmt.Errorf("nil is not a valid value for %v", target)
	}

	reflected := reflect.ValueOf(value)
	if isNilValue(reflected) {
		return reflect.Value{}, fmt.Errorf("typed nil %v is not a valid value for %v", reflected.Type(), target)
	}
	if reflected.Type().AssignableTo(target) {
		return reflected, nil
	}
	if reflected.Type().ConvertibleTo(target) {
		return reflected.Convert(target), nil
	}
	return reflect.Value{}, fmt.Errorf("value of type %v is not assignable to %v", reflected.Type(), target)
}

func isNilValue(value reflect.Value) bool {
	if !value.IsValid() {
		return true
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Ptr:
		return value.IsNil()
	default:
		return false
	}
}

func (c *Container) getInstance(componentType reflect.Type) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	instance, ok := c.instances[componentType]
	return instance, ok
}

func (c *Container) getProvider(componentType reflect.Type) (provider, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 정확한 타입 일치하는 생성자 우선 탐색
	if v, ok := c.constructors[componentType]; ok {
		return provider{typeKey: componentType, constructor: v}, nil
	}

	// 인터페이스 타입인 경우, 할당 가능한 생성자 탐색
	if componentType.Kind() == reflect.Interface {
		var matched provider
		matches := 0
		for outType, v := range c.constructors {
			if outType.AssignableTo(componentType) {
				matched = provider{typeKey: outType, constructor: v}
				matches++
			}
		}
		if matches == 1 {
			return matched, nil
		}
		if matches > 1 {
			return provider{}, fmt.Errorf("multiple constructors registered for interface %v", componentType)
		}
	}

	return provider{}, fmt.Errorf("no constructor registered for %v", componentType)
}

func (c *Container) cacheInstance(componentType reflect.Type, instance any) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.instances[componentType]; ok {
		return existing, true
	}
	c.instances[componentType] = instance
	return instance, false
}

func (c *Container) beginBuild(componentType reflect.Type) (*buildState, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.instances[componentType]; ok {
		return &buildState{instance: existing}, false, false
	}

	if state, ok := c.building[componentType]; ok {
		return state, true, true
	}

	state := &buildState{done: make(chan struct{})}
	c.building[componentType] = state
	return state, false, true
}

func (c *Container) finishBuild(componentType reflect.Type, state *buildState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if state.done != nil {
		close(state.done)
	}
	delete(c.building, componentType)
}

func formatTypePath(path []reflect.Type) string {
	parts := make([]string, len(path))
	for i, t := range path {
		parts[i] = t.String()
	}
	return strings.Join(parts, " -> ")
}

// WarmUp은 지정한 타입 목록에 대해 미리 Resolve를 호출하여 인스턴스를 생성해 둡니다.
// 이를 통해 런타임 중 초기화 비용을 분산시킬 수 있습니다.
func (c *Container) WarmUp(types []reflect.Type) error {
	seen := make(map[reflect.Type]struct{})

	for _, t := range types {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}

		// 후보 컴포넌트들을 순차적으로 인스턴스화
		if _, err := c.Resolve(t); err != nil {
			return err
		}
	}
	return nil
}
