package httperr

// InputError는 HTTP 입력 오류의 출처를 보존합니다.
// 검증 계약 자체에 HTTP 출처를 넣지 않으며 원래 오류의 식별을 유지합니다.
type InputError struct {
	Source string
	Cause  error
}

func (e *InputError) Error() string {
	if e == nil || e.Cause == nil {
		return "HTTP input error"
	}
	return e.Cause.Error()
}
func (e *InputError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
