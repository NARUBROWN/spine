package path

// Int는 전환 기간에 유지하는 기존 경로 정수 래퍼입니다.
//
// Deprecated: 일반 구조체의 int64 필드에 path 태그를 지정하고 구조체 포인터를 받으세요.
type Int struct {
	Value int64
}

// String은 전환 기간에 유지하는 기존 경로 문자열 래퍼입니다.
//
// Deprecated: 일반 구조체의 string 필드에 path 태그를 지정하고 구조체 포인터를 받으세요.
type String struct {
	Value string
}

// Boolean은 전환 기간에 유지하는 기존 경로 불리언 래퍼입니다.
//
// Deprecated: 일반 구조체의 bool 필드에 path 태그를 지정하고 구조체 포인터를 받으세요.
type Boolean struct {
	Value bool
}
