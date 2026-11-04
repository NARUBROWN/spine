package query

import (
	"strconv"
	"strings"
)

// Values는 전환 기간에 유지하는 기존 쿼리 조회 타입입니다.
// 읽는 키는 실행 중 결정되므로 상세 입력 명세를 생성할 수 없습니다.
//
// Deprecated: query 태그를 지정한 일반 구조체 포인터를 핸들러 인자로 받으세요.
type Values struct {
	values map[string][]string
}

// NewValues는 기존 조회 API에서 사용할 쿼리 뷰를 만듭니다.
//
// Deprecated: query 태그를 지정한 일반 구조체 포인터로 전환하세요.
func NewValues(values map[string][]string) Values {
	return Values{values: values}
}

// Get은 첫 번째 값을 반환하며 키나 값이 없으면 빈 문자열을 반환합니다.
//
// Deprecated: query 태그로 바인딩한 구조체 필드를 직접 읽으세요.
func (q Values) Get(key string) string {
	if v, ok := q.values[key]; ok && len(v) > 0 {
		return v[0]
	}
	return ""
}

// String은 Get과 같은 첫 번째 값을 반환합니다.
//
// Deprecated: query 태그로 바인딩한 구조체 필드를 직접 읽으세요.
func (q Values) String(key string) string {
	return q.Get(key)
}

// Int는 첫 번째 값을 정수로 읽으며 누락되거나 변환할 수 없으면 def를 반환합니다.
//
// Deprecated: 정수 필드로 바인딩하고 누락 시 기본값이 필요하면 포인터 필드를 사용하세요.
func (q Values) Int(key string, def int64) int64 {
	raw := q.Get(key)
	if raw == "" {
		return def
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return def
	}
	return v
}

// GetBoolByKey는 기존 true, false와 yes, no 및 on, off 표기를 유지합니다.
// 인식할 수 없거나 값이 없으면 def를 반환합니다.
//
// Deprecated: bool 필드 또는 필요한 형식의 TextUnmarshaler로 전환하세요.
func (q Values) GetBoolByKey(key string, def bool) bool {
	raw := strings.ToLower(q.Get(key))
	switch raw {
	case "true", "1", "yes", "y", "on":
		return true
	case "false", "0", "no", "n", "off":
		return false
	default:
		return def
	}
}

// Has는 값이 비어 있어도 키가 있으면 true를 반환합니다.
//
// Deprecated: 존재 여부가 필요하면 포인터 필드를 사용하세요.
func (q Values) Has(key string) bool {
	_, ok := q.values[key]
	return ok
}
