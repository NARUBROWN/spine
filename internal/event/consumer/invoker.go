package consumer

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/resolver"
)

type Invoker struct {
	container *container.Container
	resolvers []resolver.ArgumentResolver
}

func NewInvoker(container *container.Container, resolvers []resolver.ArgumentResolver) *Invoker {
	if container == nil {
		panic("consumer: container는 nil일 수 없습니다")
	}

	if len(resolvers) == 0 {
		panic("consumer: ArgumentResolver가 하나 이상 필요합니다")
	}

	return &Invoker{
		container: container,
		resolvers: resolvers,
	}
}

func (i *Invoker) ResolveArguments(reqCtx core.RequestContext, method reflect.Method) ([]any, error) {

	paramCount := method.Type.NumIn()
	args := make([]any, 0, paramCount-1)

	for pi := 1; pi < paramCount; pi++ {
		paramType := method.Type.In(pi)

		meta := resolver.ParameterMeta{
			Type:  paramType,
			Index: pi - 1,
		}

		resolved := false

		for _, r := range i.resolvers {
			if !r.Supports(meta) {
				continue
			}

			val, err := r.Resolve(reqCtx, meta)
			if err != nil {
				return nil, err
			}

			args = append(args, val)
			resolved = true
			break
		}

		if !resolved {
			return nil, fmt.Errorf(
				"해당 파라미터 타입을 처리할 ArgumentResolver가 없습니다: %v",
				meta.Type,
			)
		}
	}

	return args, nil
}

func (i *Invoker) Invoke(controllerType reflect.Type, method reflect.Method, args []any) ([]any, error) {

	// 컨트롤러 인스턴스 Resolve
	controller, err := i.container.Resolve(controllerType)
	if err != nil {
		return nil, err
	}

	// receiver + args → reflect.Value 배열 구성
	values := make([]reflect.Value, len(args)+1)
	values[0] = reflect.ValueOf(controller)

	for idx, arg := range args {
		values[idx+1] = reflect.ValueOf(arg)
	}

	// 메서드 호출
	results := method.Func.Call(values)

	// 결과 언래핑
	out := make([]any, len(results))
	for i, result := range results {
		out[i] = result.Interface()
	}

	return out, nil
}
