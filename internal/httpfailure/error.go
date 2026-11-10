package httpfailure

import "net/http"

type HTTPError struct {
	Status  int
	Message string
	Cause   error
}

// 오류 인터페이스의 계약 구현
func (e *HTTPError) Error() string {
	return e.Message
}

func (e *HTTPError) Unwrap() error {
	return e.Cause
}

func BadRequest(message string) error {
	return &HTTPError{Status: http.StatusBadRequest, Message: message}
}
