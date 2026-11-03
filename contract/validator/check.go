package validator

import (
	"fmt"
	"reflect"
)

// CheckValidator는 검증기를 실행하지 않고 nil 등록을 검사합니다.
// 검증 기능을 생략한 앱은 호출하지 않습니다. 타입 정보가 있는 nil도 거부합니다.
func CheckValidator(validator Validator) error {
	if validator != nil {
		value := reflect.ValueOf(validator)
		switch value.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
			if !value.IsNil() {
				return nil
			}
		default:
			return nil
		}
	}
	return fmt.Errorf("%w: validator is nil", ErrInvalidValidator)
}
