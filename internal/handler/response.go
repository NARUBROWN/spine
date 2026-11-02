package handler

import (
	"encoding/json"
	"fmt"

	"github.com/NARUBROWN/spine/core"
)

func responseWriter(ctx core.ExecutionContext) (core.ResponseWriter, error) {
	response, ok := ctx.(core.HTTPResponseContext)
	if !ok || response.ResponseWriter() == nil {
		return nil, fmt.Errorf("ResponseWriter not found in ExecutionContext")
	}
	return response.ResponseWriter(), nil
}

// ErrorJSON은 문자열 메시지만 포함하므로 직렬화할 수 없는 동적 값이 없습니다.
func ErrorJSON(message string) json.RawMessage {
	encoded, _ := json.Marshal(struct {
		Message string `json:"message"`
	}{Message: message})
	return encoded
}

func ValidateStatus(status int) error {
	if status < 100 || status > 599 {
		return fmt.Errorf("invalid HTTP response status %d", status)
	}
	return nil
}
