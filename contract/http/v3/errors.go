package httpcontract

import (
	"errors"
	"net/http"
)

var (
	ErrInvalidAdapter        = errors.New("invalid HTTP adapter")
	ErrInvalidState          = errors.New("invalid HTTP adapter state")
	ErrUnsupportedCapability = errors.New("unsupported HTTP adapter capability")
	ErrUnsupportedOption     = errors.New("unsupported HTTP adapter option")
	// ErrServerClosed는 native 엔진의 정상 Serve 종료 신호를 정규화합니다.
	ErrServerClosed = errors.New("HTTP adapter closed")
)

// RequestError는 입력 오류의 HTTP 상태와 원인을 함께 보존합니다.
// 본문 읽기 한도 초과는 413으로 나타내고 원래 읽기 오류를 Cause에 남깁니다.
// 취소와 일반 I/O 오류를 일괄 400으로 바꾸지 않습니다.
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
