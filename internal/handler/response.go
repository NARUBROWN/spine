package handler

import (
	"encoding/json"
	"fmt"

	validation "github.com/NARUBROWN/spine/contract/validator"
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

// ValidationErrorJSON은 DTO 규칙 위반만 필드별 HTTP 오류 본문으로 변환합니다.
// 취소나 구현 오류는 입력 오류로 응답하지 않습니다.
func ValidationErrorJSON(err error) (json.RawMessage, bool) {
	failure, ok := validation.AsError(err)
	if !ok {
		return nil, false
	}
	encoded, _ := json.Marshal(struct {
		Message string                  `json:"message"`
		Fields  []validation.FieldError `json:"fields"`
	}{Message: "Validation failed", Fields: failure.Fields})
	return encoded, true
}

func ValidateStatus(status int) error {
	if status < 100 || status > 599 {
		return fmt.Errorf("invalid HTTP response status %d", status)
	}
	return nil
}
