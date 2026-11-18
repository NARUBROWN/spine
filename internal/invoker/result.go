package invoker

import "reflect"

func ReturnedError(results []any) error {
	for _, result := range results {
		if IsNilResult(result) {
			continue
		}
		if err, ok := result.(error); ok {
			return err
		}
	}
	return nil
}

// IsNilResult는 명시적으로 nil을 검사합니다. error 인터페이스에 nil이 담긴 경우처럼
// 타입 정보가 있으나 값이 nil인 경우까지 포괄적으로 처리한다.
func IsNilResult(v any) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice, reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}
