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

func New() *Container {
	return &Container{
		constructors: make(map[reflect.Type]reflect.Value),
		instances:    make(map[reflect.Type]any),
		building:     make(map[reflect.Type]*buildState),
	}
}

func (c *Container) RegisterConstructor(function any) error {
	val := reflect.ValueOf(function)
	typ := val.Type()

	if typ.Kind() != reflect.Func {
		return errors.New("생성자는 함수여야 합니다")
	}

	if typ.NumOut() != 1 {
		return errors.New("생성자는 하나의 반환값만 가져야 합니다")
	}

	outType := typ.Out(0)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.constructors[outType] = val

	return nil
}

func (c *Container) Resolve(componentType reflect.Type) (any, error) {
	return c.resolve(componentType, map[reflect.Type]int{}, nil)
}

func (c *Container) resolve(componentType reflect.Type, stack map[reflect.Type]int, path []reflect.Type) (any, error) {
	if idx, ok := stack[componentType]; ok {
		cycle := append([]reflect.Type{}, path[idx:]...)
		cycle = append(cycle, componentType)
		return nil, fmt.Errorf("순환 의존성 감지: %s", formatTypePath(cycle))
	}

	if instance, ok := c.getInstance(componentType); ok {
		return instance, nil
	}

	constructor, err := c.getConstructor(componentType)
	if err != nil {
		return nil, err
	}

	state, wait, ok := c.beginBuild(componentType)
	if ok {
		if wait {
			select {
			case <-state.done:
				return state.instance, state.err
			}
		}
		defer c.finishBuild(componentType, state)
	}

	stack[componentType] = len(path)
	path = append(path, componentType)
	defer delete(stack, componentType)

	numIn := constructor.Type().NumIn()
	args := make([]reflect.Value, numIn)
	for i := 0; i < numIn; i++ {
		paramType := constructor.Type().In(i)
		paramInstance, err := c.resolve(paramType, stack, path)
		if err != nil {
			if ok && !wait {
				state.err = err
			}
			return nil, err
		}
		args[i] = reflect.ValueOf(paramInstance)
	}

	result := constructor.Call(args)[0].Interface()
	if cached, existed := c.cacheInstance(componentType, result); existed {
		if ok && !wait {
			state.instance = cached
		}
		return cached, nil
	}
	if ok && !wait {
		state.instance = result
	}

	return result, nil
}

func (c *Container) getInstance(componentType reflect.Type) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	instance, ok := c.instances[componentType]
	return instance, ok
}

func (c *Container) getConstructor(componentType reflect.Type) (reflect.Value, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 정확한 타입 일치하는 생성자 우선 탐색
	if v, ok := c.constructors[componentType]; ok {
		return v, nil
	}

	// 인터페이스 타입인 경우, 할당 가능한 생성자 탐색
	if componentType.Kind() == reflect.Interface {
		var matched reflect.Value
		matches := 0
		for outType, v := range c.constructors {
			if outType.AssignableTo(componentType) {
				matched = v
				matches++
			}
		}
		if matches == 1 {
			return matched, nil
		}
		if matches > 1 {
			return reflect.Value{}, fmt.Errorf("인터페이스 %v 에 대한 생성자가 여러 개 등록되었습니다", componentType)
		}
	}

	return reflect.Value{}, fmt.Errorf("등록된 생성자가 없습니다: %v", componentType)
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
