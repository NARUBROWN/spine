package container

import (
	"errors"
	"fmt"
	"reflect"
)

// ObjectConstructor는 의존성 인자가 서로 다른 생성 메서드의 공통 형식을 검사합니다.
// prototype은 타입을 나타냅니다. 필드 값은 복사하지 않고 호출마다 새 receiver를 만듭니다.
// 검사 중에는 생성 메서드나 의존성을 실행하지 않습니다.
func ObjectConstructor(prototype any) (any, error) {
	owner := reflect.TypeOf(prototype)
	if owner == nil || owner.Kind() != reflect.Pointer || owner.Elem().Kind() != reflect.Struct {
		return nil, errors.New("component must be a pointer to a struct with a Constructor method")
	}
	method, ok := owner.MethodByName("Constructor")
	if !ok {
		return nil, fmt.Errorf("%v must implement Constructor(dependencies...) %v", owner, owner)
	}
	if method.Type.IsVariadic() || method.Type.NumOut() != 1 || method.Type.Out(0) != owner {
		return nil, fmt.Errorf("%v.Constructor must be non-variadic and return exactly %v", owner, owner)
	}
	inputs := make([]reflect.Type, method.Type.NumIn()-1)
	for i := range inputs {
		inputs[i] = method.Type.In(i + 1)
	}
	signature := reflect.FuncOf(inputs, []reflect.Type{owner}, false)
	factory := reflect.MakeFunc(signature, func(args []reflect.Value) []reflect.Value {
		receiver := reflect.New(owner.Elem())
		return method.Func.Call(append([]reflect.Value{receiver}, args...))
	})
	return factory.Interface(), nil
}

// ValidateConstructors는 등록 전체의 중복과 의존성 그래프를 실행 없이 검사합니다.
func ValidateConstructors(constructors []any) error {
	registry := New()
	seen := map[reflect.Type]bool{}
	var outputs []reflect.Type
	var issues []error
	for index, constructor := range constructors {
		if err := registry.RegisterConstructor(constructor); err != nil {
			issues = append(issues, fmt.Errorf("constructor[%d]: %w", index, err))
			continue
		}
		output := reflect.TypeOf(constructor).Out(0)
		if seen[output] {
			issues = append(issues, fmt.Errorf("duplicate constructors for %v", output))
		}
		seen[output] = true
		outputs = append(outputs, output)
	}
	if len(issues) > 0 {
		return errors.Join(issues...)
	}
	for _, output := range outputs {
		if err := registry.validateDependencyGraph(output, map[reflect.Type]int{}, nil, map[reflect.Type]struct{}{}); err != nil {
			issues = append(issues, err)
		}
	}
	return errors.Join(issues...)
}
