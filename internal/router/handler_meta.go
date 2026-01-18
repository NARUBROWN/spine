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

	// 1. 함수인지 검증
	if t.Kind() != reflect.Func {
		return core.HandlerMeta{}, fmt.Errorf("handler는 함수여야 합니다")
	}

	// 2. 메서드 표현식인지 검증
	// 예: func(*UserController, ...)
	if t.NumIn() < 1 {
		return core.HandlerMeta{}, fmt.Errorf("handler는 메서드 표현식이어야 합니다")
	}

	receiverType := t.In(0)
	if receiverType.Kind() != reflect.Ptr {
		return core.HandlerMeta{}, fmt.Errorf("handler의 리시버는 포인터 타입이어야 합니다")
	}

	// 3. 메서드 이름 추출
	fn := runtime.FuncForPC(v.Pointer())
	if fn == nil {
		return core.HandlerMeta{}, fmt.Errorf("메서드 정보를 추출할 수 없습니다")
	}

	fullName := fn.Name()
	// 예: github.com/NARUBROWN/spine-demo.(*UserController).GetUser
	lastDot := strings.LastIndex(fullName, ".")
	if lastDot == -1 {
		return core.HandlerMeta{}, fmt.Errorf("메서드 이름 파싱 실패: %s", fullName)
	}

	methodName := fullName[lastDot+1:]

	method, ok := receiverType.MethodByName(methodName)
	if !ok {
		return core.HandlerMeta{}, fmt.Errorf("메서드를 찾을 수 없습니다: %s", methodName)
	}

	return core.HandlerMeta{
		ControllerType: receiverType,
		Method:         method,
	}, nil
}
