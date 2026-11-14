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
	Fields              []FieldSchema
	ConditionalRequired []ConditionalRequired
	Unresolved          []string
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

// ConditionalRequired는 모든 When 조건을 만족하는 객체에서 필수 필드를 지정합니다.
// Index와 When의 인덱스는 SchemaDescriber에 전달된 Go 타입을 기준으로 합니다.
type ConditionalRequired struct {
	Index  []int
	When   []FieldCondition
	Schema json.RawMessage
}

// FieldCondition은 참조 필드가 존재하고 그 값이 Value와 일치하는 조건입니다.
// Value는 JSON으로 인코딩한 단일 스칼라입니다.
type FieldCondition struct {
	Index []int
	Value json.RawMessage
}
