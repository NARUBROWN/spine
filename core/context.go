package core

// Context는 하나의 요청 실행 단위를 표현하는 공통 컨텍스트
type Context interface {
	// 경로 변수 값을 반환
	Param(name string) string
	// 쿼리 파라미터 값을 반환
	Query(name string) string
	// 요청 헤더 값을 반환
	Header(name string) string
	// 요청 본문을 주어진 구조체로 바인딩
	Bind(out any) error
	// 요청 범위 데이터 저장
	Set(key string, value any)
	// 요청 범위 데이터 조회
	Get(key string) (any, bool)

	Params() map[string]string
	Queries() map[string][]string

	JSON(status int, value any) error
	String(status int, value string) error
}
