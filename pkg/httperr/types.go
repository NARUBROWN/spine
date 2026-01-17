package httperr

type HTTPError struct {
	Status  int
	Message string
	Cause   error
}

// error 인터페이스의 계약 구현
func (e *HTTPError) Error() string {
	return e.Message
}

func NotFound(msg string) error {
	return &HTTPError{Status: 404, Message: msg}
}

func BadRequest(msg string) error {
	return &HTTPError{Status: 400, Message: msg}
}

func Unauthorized(msg string) error {
	return &HTTPError{Status: 401, Message: msg}
}
