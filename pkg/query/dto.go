package query

// DTO는 query 태그로 선언한 T의 필드를 쿼리에서만 바인딩합니다.
// T는 구조체여야 하며 바인딩 뒤 등록된 DTO 검증기를 적용합니다.
// JSON 본문이 같은 필드의 쿼리 값을 덮어쓰지 않습니다.
type DTO[T any] struct{ Value T }
