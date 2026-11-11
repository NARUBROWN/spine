package query

// Pagination은 전환 기간에 유지하는 기존 page, size 쿼리 입력입니다.
// 누락되거나 정수 변환에 실패하면 Page는 1, Size는 20을 사용합니다.
//
// Deprecated: page, size에 query 태그를 지정한 DTO를 input.Query[T]로 받으세요.
// 누락 시 기본값은 핸들러에서 적용하고 값 제약은 선택한 validator에 선언하세요.
type Pagination struct {
	Page int
	Size int
}
