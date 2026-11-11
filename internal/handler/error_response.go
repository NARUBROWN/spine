package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/internal/event/hook"
	"github.com/NARUBROWN/spine/internal/httpfailure"
)

// WriteErrorResponse는 오류가 발생한 위치와 무관하게 같은 HTTP 응답 규칙을 적용합니다.
// 명시적인 HTTPError는 일반 오류 분류보다 우선하며 400~599 상태만 허용합니다.
func WriteErrorResponse(writer httpcontract.ResponseWriter, err error) error {
	status := http.StatusInternalServerError
	message := "Internal server error"
	var responseErr error
	var body json.RawMessage
	var httpErr *httpfailure.HTTPError
	if errors.As(err, &httpErr) && httpErr != nil {
		if httpErr.Status < http.StatusBadRequest || httpErr.Status > 599 {
			responseErr = fmt.Errorf("invalid HTTP error status %d", httpErr.Status)
		} else {
			status, message = httpErr.Status, httpErr.Message
		}
	} else if errors.Is(err, hook.ErrPublisherUnavailable) {
		status, message = http.StatusServiceUnavailable, hook.ErrPublisherUnavailable.Error()
	} else if validationBody, ok := ValidationErrorJSON(err); ok {
		status, body = http.StatusBadRequest, validationBody
	} else {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			status, message = http.StatusRequestEntityTooLarge, "Request entity too large"
		}
	}
	if body == nil {
		body = InputErrorJSON(err, status, message)
	}
	if writeErr := writer.WriteJSON(status, body); writeErr != nil {
		responseErr = errors.Join(responseErr, fmt.Errorf("write execution error response: %w", writeErr))
	}
	return responseErr
}
