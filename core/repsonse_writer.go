package core

// ResponseWriter는 transport(Echo, net/http 등)에 의존하지 않는
// Spine의 응답 출력 계약이다.
// 실제 구현은 adapter 레이어에서 제공한다.
type ResponseWriter interface {
	// Header 조작
	SetHeader(key, value string)

	// 상태 코드만 기록 (body 없음)
	WriteStatus(status int) error

	// body + status 기록
	WriteJSON(status int, value any) error
	WriteString(status int, value string) error
}
