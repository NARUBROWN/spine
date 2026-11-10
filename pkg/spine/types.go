package spine

// Ctx는 핸들러가 인터셉터에서 설정한 요청 값을 읽는 인터페이스입니다.
type Ctx interface {
	Get(key string) (any, bool)
}
