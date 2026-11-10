package resolver

import (
	"reflect"

	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/runtime"
)

type ControllerContextResolver struct {
	// RequireExecutionContext는 전체 핸들러 컨텍스트 계약을 선언한 인자만 받습니다.
	RequireExecutionContext bool
}

func (r *ControllerContextResolver) Supports(parameterMeta ParameterMeta) bool {
	t := parameterMeta.Type
	if t == nil || t.Kind() != reflect.Interface {
		return false
	}
	if r.RequireExecutionContext && !t.Implements(reflect.TypeFor[runtime.ControllerContext]()) {
		return false
	}

	// Resolve가 실제로 반환하는 concrete view가 요청된 인터페이스에
	// 할당될 수 있을 때만 지원한다고 광고합니다. ControllerContext를
	// 확장한 사용자 인터페이스는 추가 메서드를 구현하지 않으므로 제외됩니다.
	resolvedType := reflect.TypeOf(runtime.NewControllerContext(nil))
	return resolvedType.AssignableTo(t)
}

func (r *ControllerContextResolver) Resolve(ctx execution.Context, _ ParameterMeta) (any, error) {
	return runtime.NewControllerContext(ctx), nil
}
