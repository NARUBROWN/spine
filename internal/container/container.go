package container

import (
	"errors"
	"fmt"
	"log"
	"reflect"
)

type Container struct {
	constructors map[reflect.Type]reflect.Value
	instances    map[reflect.Type]any
	creating     map[reflect.Type]bool
}

func New() *Container {
	return &Container{
		constructors: make(map[reflect.Type]reflect.Value),
		instances:    make(map[reflect.Type]any),
		creating:     make(map[reflect.Type]bool),
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
	c.constructors[outType] = val

	return nil
}

func (c *Container) Resolve(componentType reflect.Type) (any, error) {
	if instance, ok := c.instances[componentType]; ok {
		return instance, nil
	}

	if c.creating[componentType] {
		return nil, fmt.Errorf("순환 의존성 감지: %v", componentType)
	}

	constructor, hasConstructor := c.constructors[componentType]
	if !hasConstructor {
		return nil, fmt.Errorf("등록된 생성자가 없습니다: %v", componentType)
	}

	c.creating[componentType] = true
	defer delete(c.creating, componentType)

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

	result := constructor.Call(args)[0].Interface()
	c.instances[componentType] = result

	return result, nil
}

func (c *Container) WarmUp(types []reflect.Type) error {
	seen := make(map[reflect.Type]struct{})

	for _, t := range types {
		log.Printf("[Container] Instance에 의존성 후보 등록 : %s", t.Elem().Name())
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}

		// 차례대로 후보 컴포넌트의 Resolve 호출하여 instance화
		if _, err := c.Resolve(t); err != nil {
			return err
		}
	}
	return nil
}
