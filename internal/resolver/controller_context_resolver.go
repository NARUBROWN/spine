package resolver

import (
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/runtime"
)

type ControllerContextResolver struct{}

func (r *ControllerContextResolver) Supports(parameterMeta ParameterMeta) bool {
	t := parameterMeta.Type
	if t == nil || t.Kind() != reflect.Interface {
		return false
	}

	// 실제 읽기 전용 뷰를 할당할 수 있는 인터페이스만 지원합니다.
	// Ctx에 메서드를 추가한 인터페이스는 이 뷰가 구현하지 않습니다.
	resolvedType := reflect.TypeOf(runtime.NewControllerContext(nil))
	return resolvedType.AssignableTo(t)
}

func (r *ControllerContextResolver) Resolve(ctx core.ExecutionContext, _ ParameterMeta) (any, error) {
	return runtime.NewControllerContext(ctx), nil
}
