package core

import (
	"reflect"
)

// HandlerMeta는 인터셉터에 전달하는 핸들러 정보의 사본입니다.
type HandlerMeta struct {
	ControllerType   reflect.Type
	MethodName       string
	Signature        reflect.Type
	PathKeys         []string
	InterceptorTypes []reflect.Type
	CallbackRoute    bool
}

func (m HandlerMeta) FunctionType() reflect.Type { return m.Signature }

func (m HandlerMeta) ArgumentOffset() int {
	if m.CallbackRoute {
		return 0
	}
	return 1
}

func (m HandlerMeta) HandlerName() string {
	if m.ControllerType == nil {
		return ""
	}
	if m.CallbackRoute {
		return m.ControllerType.String() + ".RegisterRoutes"
	}
	return m.ControllerType.String() + "." + m.MethodName
}
