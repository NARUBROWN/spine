package execution

import (
	"reflect"
	"slices"

	"github.com/NARUBROWN/spine/core"
)

// HandlerMeta는 실제로 실행할 핸들러에 대한 메타데이터입니다.
type HandlerMeta struct {
	// 컨트롤러 타입(Container의 Resolve 대상)
	ControllerType reflect.Type
	// 호출할 메서드 이름
	Method reflect.Method
	// Callback은 DI 인스턴스를 캡처한 receiver 없는 함수입니다.
	Callback reflect.Value
	// 라우트에 선언된 경로 매개변수 키의 순서
	PathKeys []string
	// 핸들러에 적용된 인터셉터
	Interceptors []core.Interceptor
}

// FunctionType과 ArgumentOffset은 실행과 명세가 공유하는 호출 서명입니다.
func (m HandlerMeta) FunctionType() reflect.Type {
	if m.Callback.IsValid() {
		return m.Callback.Type()
	}
	return m.Method.Type
}
func (m HandlerMeta) ArgumentOffset() int {
	if m.Callback.IsValid() {
		return 0
	}
	return 1
}
func (m HandlerMeta) HandlerName() string {
	if m.ControllerType == nil {
		return ""
	}
	if m.Callback.IsValid() {
		return m.ControllerType.String() + ".RegisterRoutes"
	}
	return m.ControllerType.String() + "." + m.Method.Name
}

func (m HandlerMeta) View() core.HandlerMeta {
	var interceptorTypes []reflect.Type
	if len(m.Interceptors) != 0 {
		interceptorTypes = make([]reflect.Type, len(m.Interceptors))
		for index, interceptor := range m.Interceptors {
			interceptorTypes[index] = reflect.TypeOf(interceptor)
		}
	}
	return core.HandlerMeta{
		ControllerType:   m.ControllerType,
		MethodName:       m.Method.Name,
		Signature:        m.FunctionType(),
		PathKeys:         slices.Clone(m.PathKeys),
		InterceptorTypes: interceptorTypes,
		CallbackRoute:    m.Callback.IsValid(),
	}
}
