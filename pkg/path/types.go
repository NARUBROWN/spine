package path

// Int는 전환 기간에 유지하는 기존 경로 정수 래퍼입니다.
//
// Deprecated: input.Path[int64] 또는 path 태그를 지정한 DTO의 input.Path[T]를 받으세요.
type Int struct {
	Value int64
}

// String은 전환 기간에 유지하는 기존 경로 문자열 래퍼입니다.
//
// Deprecated: input.Path[string] 또는 path 태그를 지정한 DTO의 input.Path[T]를 받으세요.
type String struct {
	Value string
}

// Boolean은 전환 기간에 유지하는 기존 경로 불리언 래퍼입니다.
//
// Deprecated: input.Path[bool] 또는 path 태그를 지정한 DTO의 input.Path[T]를 받으세요.
type Boolean struct {
	Value bool
}
