package router

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"github.com/NARUBROWN/spine/core"
)

// NewHandlerMeta는 메서드 표현식 (*Controller).Method 를
// 실행 가능한 HandlerMeta로 변환합니다.
func NewHandlerMeta(handler any) (core.HandlerMeta, error) {
	t := reflect.TypeOf(handler)
	v := reflect.ValueOf(handler)
	if t == nil {
		return core.HandlerMeta{}, fmt.Errorf("handler must not be nil")
	}

	// 1. 함수인지 검증
	if t.Kind() != reflect.Func || v.IsNil() {
		return core.HandlerMeta{}, fmt.Errorf("handler must be a function")
	}

	// 2. 메서드 표현식인지 검증
	// 예: func(*UserController, ...)
	if t.NumIn() < 1 {
		return core.HandlerMeta{}, fmt.Errorf("handler must be a method expression")
	}

	receiverType := t.In(0)
	if receiverType.Kind() != reflect.Ptr {
		return core.HandlerMeta{}, fmt.Errorf("handler receiver must be a pointer type")
	}

	// 3. 메서드 이름 추출
	fn := runtime.FuncForPC(v.Pointer())
	if fn == nil {
		return core.HandlerMeta{}, fmt.Errorf("cannot extract method information")
	}

	fullName := fn.Name()
	// 예: github.com/NARUBROWN/spine-demo.(*UserController).GetUser
	lastDot := strings.LastIndex(fullName, ".")
	if lastDot == -1 {
		return core.HandlerMeta{}, fmt.Errorf("failed to parse method name: %s", fullName)
	}

	methodName := fullName[lastDot+1:]

	method, ok := receiverType.MethodByName(methodName)
	if !ok {
		return core.HandlerMeta{}, fmt.Errorf("method not found: %s", methodName)
	}

	return core.HandlerMeta{
		ControllerType: receiverType,
		Method:         method,
	}, nil
}

// Meta는 기존 메서드 표현식과 컨트롤러 콜백을 구분합니다.
func Meta(spec RouteSpec) (core.HandlerMeta, error) {
	if spec.ControllerType == nil {
		return NewHandlerMeta(spec.Handler)
	}
	value := reflect.ValueOf(spec.Handler)
	if !value.IsValid() || value.Kind() != reflect.Func || value.IsNil() {
		return core.HandlerMeta{}, fmt.Errorf("callback must be a non-nil function")
	}
	if value.Type().IsVariadic() {
		return core.HandlerMeta{}, fmt.Errorf("variadic callbacks are not supported")
	}
	return core.HandlerMeta{ControllerType: spec.ControllerType, Callback: value}, nil
}
