// Package input은 핸들러 인자의 HTTP 입력 출처를 선언합니다.
package input

// Path는 경로 값을 T에 바인딩합니다. T는 구조체 또는 기본 스칼라입니다.
// 스칼라는 경로 변수가 정확히 하나인 라우트에서만 사용할 수 있습니다.
type Path[T any] *T

// Query는 쿼리 값을 T의 필드에 바인딩합니다. T는 구조체여야 합니다.
type Query[T any] *T

// Body는 요청 본문을 T에 바인딩합니다. T는 구조체여야 합니다.
type Body[T any] *T
