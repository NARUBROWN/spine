package header

import "net/http"

// Values는 전환 기간에 유지하는 기존 HTTP 헤더 조회 타입입니다.
// 읽는 키는 실행 중 결정되므로 상세 입력 명세를 생성할 수 없습니다.
//
// Deprecated: header 태그를 지정한 DTO를 input.Header[T]로 받으세요.
type Values struct {
	headers http.Header
}

// NewValues는 주어진 헤더로 기존 조회 뷰를 만듭니다.
//
// Deprecated: header 태그를 지정한 DTO와 input.Header[T]로 전환하세요.
func NewValues(headers http.Header) Values {
	return Values{headers: headers}
}

// Get은 http.Header와 같은 키 비교 규칙으로 첫 번째 값을 반환합니다.
//
// Deprecated: header 태그로 바인딩한 구조체 필드를 직접 읽으세요.
func (h Values) Get(key string) string {
	return h.headers.Get(key)
}

// Has는 빈 값도 포함해 키의 존재 여부를 반환합니다.
// 키 비교는 http.Header와 같습니다.
//
// Deprecated: 존재 여부가 필요하면 포인터 필드를 사용하세요.
func (h Values) Has(key string) bool {
	_, ok := h.headers[http.CanonicalHeaderKey(key)]
	return ok
}
