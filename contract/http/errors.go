package httpcontract

import (
	"errors"
	"net/http"
)

var (
	// ErrInvalidAdapter는 nil 객체, 잘못된 구성 또는 규약 버전 불일치를 나타냅니다.
	ErrInvalidAdapter = errors.New("invalid HTTP adapter")
	// ErrInvalidState는 중복 실행이나 종료 후 재실행 등 허용되지 않는 호출 상태를 나타냅니다.
	ErrInvalidState = errors.New("invalid HTTP adapter state")
	// ErrUnsupportedCapability는 선택한 어댑터가 요청한 기능을 지원하지 않음을 나타냅니다.
	ErrUnsupportedCapability = errors.New("unsupported HTTP adapter capability")
)

// RequestError는 어댑터가 요청 입력 오류의 HTTP 상태와 원인을 보존하는 오류입니다.
// 커널은 이를 자신의 공개 오류 표현으로 변환할 수 있습니다.
type RequestError struct {
	Status  int
	Message string
	Cause   error
}

func (e *RequestError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return http.StatusText(e.Status)
}

func (e *RequestError) Unwrap() error { return e.Cause }
