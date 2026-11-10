package httperr

import "github.com/NARUBROWN/spine/internal/httpfailure"

// HTTPError는 애플리케이션이 HTTP 상태와 응답 메시지를 지정할 때 사용하는 오류입니다.
type HTTPError = httpfailure.HTTPError

func NotFound(msg string) error {
	return &HTTPError{Status: 404, Message: msg}
}

func BadRequest(msg string) error {
	return httpfailure.BadRequest(msg)
}

func Unauthorized(msg string) error {
	return &HTTPError{Status: 401, Message: msg}
}

func InternalServerError(msg string) error {
	return &HTTPError{Status: 500, Message: msg}
}
