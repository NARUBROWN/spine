package validator

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// ErrInvalidValue는 DTO로 지원하지 않는 값이나 nil DTO를 전달했음을 나타냅니다.
// 요청 필드의 규칙 위반과 구분하며, 구현은 오류를 감싸더라도 errors.Is로 식별할 수 있게 합니다.
var ErrInvalidValue = errors.New("invalid validation value")

// ErrInvalidValidator는 nil 등록이나 비어 있는 검증 실패 등 계약을 위반한 구현을 나타냅니다.
var ErrInvalidValidator = errors.New("invalid validator")

// FieldError는 입력값을 포함하지 않는 필드별 규칙 위반입니다.
// 반환된 필드 오류와 Error.Fields는 호출자가 읽기 전용으로 사용합니다.
type FieldError struct {
	// Field는 외부 필드 이름을 사용하는 JSON Pointer입니다. 예: /items/0/email.
	// 전체 DTO에 대한 오류는 빈 문자열을 사용합니다. '~'와 '/'는 각각 '~0'과 '~1'로 이스케이프합니다.
	Field string `json:"field"`
	// Rule은 구현이 문서화한 기계 판독용 규칙 이름입니다. 예: required, email, gte.
	Rule string `json:"rule"`
	// Param은 규칙의 설정값입니다. 예: gte=18의 "18". 거부된 입력값은 넣지 않습니다.
	Param string `json:"param,omitempty"`
	// Message는 표시용 설명입니다. 프로그램의 오류 분류에는 Rule을 사용합니다.
	Message string `json:"message,omitempty"`
}

// Error는 DTO 규칙 위반을 모읍니다. errors.As로 감싸진 오류에서도 확인할 수 있습니다.
// Fields에는 오류가 하나 이상 있어야 하며, 구현은 같은 입력에 대해 같은 순서를 유지합니다.
// HTTP 상태, 브로커 재시도 정책과 WebSocket 종료 정책은 호출자가 결정합니다.
type Error struct {
	Fields []FieldError `json:"fields"`
}

func (e *Error) Error() string {
	if e == nil || len(e.Fields) == 0 {
		return "validation failed"
	}
	return fmt.Sprintf("validation failed with %d field error(s)", len(e.Fields))
}

// NewError는 필드 오류가 없으면 nil을, 있으면 목록을 복사한 *Error를 반환합니다.
// 순서와 같은 필드의 여러 규칙 위반을 그대로 보존합니다.
func NewError(fields ...FieldError) error {
	if len(fields) == 0 {
		return nil
	}
	return &Error{Fields: slices.Clone(fields)}
}

// AsError는 감싸진 오류에서 유효한 DTO 규칙 위반을 꺼냅니다.
// 취소, 구현 오류와 비어 있는 실패는 입력 오류로 분류하지 않습니다.
func AsError(err error) (*Error, bool) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, ErrInvalidValidator) || errors.Is(err, ErrInvalidValue) {
		return nil, false
	}
	var failure *Error
	if !errors.As(err, &failure) || failure == nil || len(failure.Fields) == 0 {
		return nil, false
	}
	return failure, true
}
