package core

import (
	"reflect"
)

// HandlerMeta는 실제로 실행할 핸들러에 대한 메타데이터입니다.
type HandlerMeta struct {
	// 컨트롤러 타입 (Container Resolve 대상)
	ControllerType reflect.Type
	// 호출할 메서드 이름
	Method reflect.Method
}
