package invoker

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/handler"
	"github.com/NARUBROWN/spine/internal/resolver"
)

type Invoker struct {
	container      *container.Container
	argRegistry    *resolver.Registry
	returnRegistry *handler.ReturnHandlerRegistry
}

func NewInvoker(container *container.Container, argRegistry *resolver.Registry, returnRegistry *handler.ReturnHandlerRegistry) *Invoker {
	return &Invoker{
		container:      container,
		argRegistry:    argRegistry,
		returnRegistry: returnRegistry,
	}
}

/*
Invoke는 컨트롤러 타입과 메서드 이름을 받아 해당 메서드를 실행한다.
현재는 인자가 없는 메서드만 지원한다.
*/
func (i *Invoker) Invoke(controllerType reflect.Type, methodName string, ctx core.Context) error {
	// 컨트롤러 인스턴스 Resolve
	controller, err := i.container.Resolve(controllerType)
	if err != nil {
		return err
	}

	controllerValue := reflect.ValueOf(controller)

	// 메서드 조회
	method := controllerValue.MethodByName(methodName)
	if !method.IsValid() {
		return fmt.Errorf(
			"메서드를 찾을 수 없습니다: %s.%s",
			controllerType.String(),
			methodName,
		)
	}

	methodType := method.Type()

	// 파라미터 해석
	args := make([]reflect.Value, methodType.NumIn())

	for index := 0; index < methodType.NumIn(); index++ {
		paramType := methodType.In(index)

		fmt.Println("PARAM TYPE:", paramType.String())

		arg, err := i.argRegistry.Resolve(paramType, ctx)
		if err != nil {
			fmt.Println("ARG RESOLVE ERROR:", paramType, err)
			return err
		}

		args[index] = reflect.ValueOf(arg)
	}

	// 메서드 호출
	results := method.Call(args)

	values := make([]any, len(results))
	for i, result := range results {
		values[i] = result.Interface()
	}

	return i.returnRegistry.Handle(values, ctx)
}
