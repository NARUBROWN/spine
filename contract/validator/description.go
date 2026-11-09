package validator

import (
	"encoding/json"
	"reflect"
)

// SchemaDescriber는 등록된 검증기의 규칙을 설명하는 선택적 계약입니다.
// Validate, 사용자 규칙, 생성자와 네트워크를 실행하지 않습니다.
// 한 인스턴스를 동시에 호출할 수 있어야 하며 실제 검증 설정만 설명합니다.
// 지원하지 않는 규칙은 Unresolved에 남기고 검증 성공 여부를 추측하지 않습니다.
type SchemaDescriber interface {
	DescribeSchema(reflect.Type) (SchemaDescription, error)
}

type SchemaDescription struct {
	Fields     []FieldSchema
	Unresolved []string
}

// FieldSchema는 Go 필드 인덱스로 검증 제약을 연결합니다.
// Schema는 JSON Schema 2020-12의 객체이며 필드명과 구조는 명세 생성기가 결정합니다.
// Required는 입력에서 이 필드가 생략될 수 없음을 뜻합니다.
// 포인터의 null, 숫자 0, bool false 허용 여부는 Schema에도 정확히 표현합니다.
type FieldSchema struct {
	Index    []int
	Required bool
	Schema   json.RawMessage
	// SkipNested는 이 필드 내부의 DTO 검증 규칙을 적용하지 않음을 나타냅니다.
	SkipNested bool
}
