package validator_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	validation "github.com/NARUBROWN/spine/contract/validator"
)

type functionValidator func(context.Context, any) error
type mapValidator map[string]string
type sliceValidator []string
type channelValidator chan struct{}
type valueValidator struct{}

func (functionValidator) Validate(context.Context, any) error { panic("unexpected validation") }
func (mapValidator) Validate(context.Context, any) error      { panic("unexpected validation") }
func (sliceValidator) Validate(context.Context, any) error    { panic("unexpected validation") }
func (channelValidator) Validate(context.Context, any) error  { panic("unexpected validation") }
func (valueValidator) Validate(context.Context, any) error    { panic("unexpected validation") }

func TestCheckValidatorRejectsNilWithoutCallingValidation(t *testing.T) {
	for _, validator := range []validation.Validator{
		nil, (*nameValidator)(nil), functionValidator(nil), mapValidator(nil), sliceValidator(nil), channelValidator(nil),
	} {
		if err := validation.CheckValidator(validator); !errors.Is(err, validation.ErrInvalidValidator) {
			t.Fatalf("nil 검증기는 호출 전에 거부해야 합니다: %T %v", validator, err)
		}
	}
	for _, validator := range []validation.Validator{
		&nameValidator{}, functionValidator(func(context.Context, any) error { panic("unexpected validation") }),
		mapValidator{}, sliceValidator{}, make(channelValidator), valueValidator{},
	} {
		if err := validation.CheckValidator(validator); err != nil {
			t.Fatalf("검증기를 실행하지 않고 등록 가능 여부만 확인해야 합니다: %T %v", validator, err)
		}
	}
}

func TestAsErrorDistinguishesValidationFailureFromProcessingFailure(t *testing.T) {
	failure := validation.NewError(validation.FieldError{Field: "/age", Rule: "gte", Param: "18"})
	for _, err := range []error{failure, fmt.Errorf("wrapped: %w", failure)} {
		if details, ok := validation.AsError(err); !ok || len(details.Fields) != 1 {
			t.Fatalf("감싸진 규칙 위반의 필드 정보를 반환해야 합니다: %v", err)
		}
	}
	for _, err := range []error{
		nil, (*validation.Error)(nil), &validation.Error{}, errors.New("processing failure"),
		errors.Join(failure, context.Canceled), errors.Join(failure, context.DeadlineExceeded),
		errors.Join(failure, validation.ErrInvalidValue), errors.Join(failure, validation.ErrInvalidValidator),
	} {
		if details, ok := validation.AsError(err); ok || details != nil {
			t.Fatalf("취소나 구현 오류를 입력 오류로 분류하면 안 됩니다: %v", err)
		}
	}
}
