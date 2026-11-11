package handler

import (
	"encoding/json"
	"fmt"
	"github.com/NARUBROWN/spine/internal/inputerr"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/execution"
)

func responseWriter(ctx execution.Context) (httpcontract.ResponseWriter, error) {
	response, ok := ctx.(execution.HTTPResponseContext)
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

// InputErrorJSON은 외부에 공개할 HTTP 입력 오류에만 출처를 덧붙입니다.
func InputErrorJSON(err error, status int, message string) json.RawMessage {
	source, _ := inputerr.Source(err)
	if status < 400 || status >= 500 || source == "" {
		return ErrorJSON(message)
	}
	encoded, _ := json.Marshal(struct {
		Message string `json:"message"`
		In      string `json:"in"`
	}{Message: message, In: source})
	return encoded
}

// ValidationErrorJSON은 DTO 규칙 위반만 필드별 HTTP 오류 본문으로 변환합니다.
// 취소나 구현 오류는 입력 오류로 응답하지 않습니다.
func ValidationErrorJSON(err error) (json.RawMessage, bool) {
	failure, ok := validation.AsError(err)
	if !ok {
		return nil, false
	}
	type field struct {
		validation.FieldError
		In string `json:"in,omitempty"`
	}
	source, _ := inputerr.Source(err)
	fields := make([]field, len(failure.Fields))
	for i, original := range failure.Fields {
		fields[i] = field{FieldError: original, In: source}
	}
	encoded, _ := json.Marshal(struct {
		Message string  `json:"message"`
		Fields  []field `json:"fields"`
	}{Message: "Validation failed", Fields: fields})
	return encoded, true
}

func ValidateStatus(status int) error {
	if status < 100 || status > 599 {
		return fmt.Errorf("invalid HTTP response status %d", status)
	}
	return nil
}
