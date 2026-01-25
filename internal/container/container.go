package container

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
)

type Container struct {
	mu           sync.RWMutex
	constructors map[reflect.Type]reflect.Value
	instances    map[reflect.Type]any
}

func New() *Container {
	return &Container{
		constructors: make(map[reflect.Type]reflect.Value),
		instances:    make(map[reflect.Type]any),
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
	c.mu.RLock()
	// 이미 생성된 인스턴스는 락 없이 반환
	instance, ok := c.instances[componentType]
	c.mu.RUnlock()
	if ok {
		return instance, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Write lock 획득 후에도 이미 생성된 인스턴스가 있으면 그대로 반환
	if instance, ok := c.instances[componentType]; ok {
		return instance, nil
	}

	var constructor reflect.Value
	hasConstructor := false

	// 정확한 타입 일치하는 생성자 우선 탐색
	if v, ok := c.constructors[componentType]; ok {
		constructor = v
		hasConstructor = true
	}

	// 인터페이스 타입인 경우, 할당 가능한 생성자 탐색
	if !hasConstructor && componentType.Kind() == reflect.Interface {
		for outType, v := range c.constructors {
			if outType.AssignableTo(componentType) {
				constructor = v
				hasConstructor = true
				break
			}
		}
	}

	if !hasConstructor {
		return nil, fmt.Errorf("등록된 생성자가 없습니다: %v", componentType)
	}

	numIn := constructor.Type().NumIn()
	args := make([]reflect.Value, numIn)
	for i := 0; i < numIn; i++ {
		paramType := constructor.Type().In(i)
		paramInstance, err := c.Resolve(paramType)
		if err != nil {
			return nil, err
		}
		args[i] = reflect.ValueOf(paramInstance)
	}

	// 생성자 호출하여 인스턴스 생성 후 캐싱
	result := constructor.Call(args)[0].Interface()
	c.instances[componentType] = result

	return result, nil
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
