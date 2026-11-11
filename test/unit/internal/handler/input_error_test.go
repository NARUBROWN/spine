package handler

import (
	"encoding/json"
	"errors"
	"github.com/NARUBROWN/spine/internal/inputerr"
	"testing"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/pkg/httperr"
)

func TestHTTPValidationErrorsExposeSourceWithoutMutatingSharedFailures(t *testing.T) {
	failure := validation.NewError(validation.FieldError{Field: "/x-id/0", Rule: "required"})
	err := &inputerr.Error{Source: "header", Cause: failure}
	data, ok := ValidationErrorJSON(err)
	var body struct {
		Fields []struct{ Field, In, Rule string } `json:"fields"`
	}
	if decodeErr := json.Unmarshal(data, &body); !ok || decodeErr != nil || len(body.Fields) != 1 || body.Fields[0].In != "header" || body.Fields[0].Field != "/x-id/0" {
		t.Fatalf("HTTP 출처와 실제 헤더 이름: %s %v", data, decodeErr)
	}
	if !errors.Is(err, failure) || failure.(*validation.Error).Fields[0].Field != "/x-id/0" {
		t.Fatal("원래 검증 오류를 변경하면 안 됩니다")
	}
	plain, _ := ValidationErrorJSON(failure)
	var raw map[string]any
	_ = json.Unmarshal(plain, &raw)
	if _, exists := raw["fields"].([]any)[0].(map[string]any)["in"]; exists {
		t.Fatal("출처를 모르는 일반 검증 오류의 위치를 추측하면 안 됩니다")
	}
}

func TestBindingErrorsIncludeSourceButProcessingFailuresStayGeneric(t *testing.T) {
	err := &inputerr.Error{Source: "query", Cause: httperr.BadRequest("invalid query field age")}
	var body map[string]any
	_ = json.Unmarshal(InputErrorJSON(err, 400, "invalid query field age"), &body)
	if body["in"] != "query" {
		t.Fatal("바인딩 오류의 출처가 필요합니다")
	}
	body = nil
	_ = json.Unmarshal(InputErrorJSON(err, 500, "Internal server error"), &body)
	if body["in"] != nil || body["message"] != "Internal server error" {
		t.Fatal("처리 오류를 입력 필드 오류로 표시하면 안 됩니다")
	}
}
