package header

import "net/http"

// Values는 HTTP 헤더를 나타내는 타입입니다.
type Values struct {
	headers http.Header
}

// NewValues는 주어진 헤더로 새 Values 인스턴스를 생성합니다.
func NewValues(headers http.Header) Values {
	return Values{headers: headers}
}

// Get은 키에 해당하는 헤더 값을 반환합니다.
func (h Values) Get(key string) string {
	return h.headers.Get(key)
}

// Has는 키에 해당하는 헤더가 있는지 확인합니다.
// 값이 빈 문자열이어도 키가 있으면 true입니다. 키 비교는 http.Header와 같습니다.
func (h Values) Has(key string) bool {
	_, ok := h.headers[http.CanonicalHeaderKey(key)]
	return ok
}
