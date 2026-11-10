package jsonx

import "github.com/NARUBROWN/spine/internal/jsondecode"

// NullError는 nil을 표현할 수 없는 일반 값 타입에 JSON null을 전달한 오류입니다.
type NullError = jsondecode.NullError

// Unmarshal은 일반 값의 JSON null을 검사한 뒤 입력을 디코딩합니다.
// 포인터, map, slice, interface 및 사용자 UnmarshalJSON의 null 처리는 유지합니다.
func Unmarshal(data []byte, target any) error {
	return jsondecode.Unmarshal(data, target)
}
