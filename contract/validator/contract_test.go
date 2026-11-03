package validator_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	validation "github.com/NARUBROWN/spine/contract/validator"
)

// 외부 구현이 내부 패키지 없이 공개 인터페이스를 구현할 수 있는지 확인합니다.
var _ validation.Validator = (*nameValidator)(nil)

type nameDTO struct {
	Name string `json:"name"`
}

// nameValidator는 실제 검증 라이브러리 대신 공개 계약의 사용법을 보여주는 예제입니다.
type nameValidator struct{}

func (*nameValidator) Validate(ctx context.Context, value any) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("name validation: %w", err)
	}
	var dto nameDTO
	switch typed := value.(type) {
	case nameDTO:
		dto = typed
	case *nameDTO:
		if typed == nil {
			return fmt.Errorf("nil *nameDTO: %w", validation.ErrInvalidValue)
		}
		dto = *typed
	default:
		return fmt.Errorf("expected nameDTO or *nameDTO: %w", validation.ErrInvalidValue)
	}
	if dto.Name == "" {
		return validation.NewError(validation.FieldError{Field: "/name", Rule: "required"})
	}
	return nil
}

func ExampleValidator() {
	var validator validation.Validator = &nameValidator{}
	err := validator.Validate(context.Background(), &nameDTO{})

	var failure *validation.Error
	if errors.As(err, &failure) {
		fmt.Println(failure.Fields[0].Field, failure.Fields[0].Rule)
	}

	fmt.Println(validator.Validate(context.Background(), nameDTO{Name: "Kim"}))
	fmt.Println(errors.Is(validator.Validate(context.Background(), nil), validation.ErrInvalidValue))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fmt.Println(errors.Is(validator.Validate(ctx, &nameDTO{}), context.Canceled))
	// Output:
	// /name required
	// <nil>
	// true
	// true
}

func TestNewErrorReturnsNilWithoutFieldErrors(t *testing.T) {
	for _, fields := range [][]validation.FieldError{nil, {}} {
		if err := validation.NewError(fields...); err != nil {
			t.Fatalf("필드 오류가 없으면 error 인터페이스 자체가 nil이어야 합니다: %#v", err)
		}
	}
}

func TestNewErrorOwnsFieldListAndPreservesWrappedDetails(t *testing.T) {
	fields := []validation.FieldError{
		{Field: "/items/0/age", Rule: "gte", Param: "18", Message: "18세 이상이어야 합니다"},
		{Field: "/items/0/age", Rule: "custom_age"},
		{Field: "", Rule: "consistent"},
	}
	want := append([]validation.FieldError(nil), fields...)
	err := validation.NewError(fields...)
	fields[0] = validation.FieldError{Field: "/changed", Rule: "changed"}

	var failure *validation.Error
	if !errors.As(fmt.Errorf("DTO validation: %w", err), &failure) {
		t.Fatalf("상위 오류로 감싸도 공통 검증 오류를 확인할 수 있어야 합니다: %v", err)
	}
	if !reflect.DeepEqual(failure.Fields, want) {
		t.Fatalf("원본 변경과 관계없이 순서, 중복 필드와 전체 DTO 오류를 보존해야 합니다: %+v", failure.Fields)
	}
	if err.Error() != "validation failed with 3 field error(s)" {
		t.Fatalf("오류 요약에 필드 오류 개수가 있어야 합니다: %v", err)
	}
	if strings.Contains(err.Error(), want[0].Message) {
		t.Fatalf("표시용 설명을 오류 요약에 자동으로 포함하면 안 됩니다: %v", err)
	}
}

func TestFieldErrorsJSON(t *testing.T) {
	err := validation.NewError(
		validation.FieldError{Field: "/items/0/email", Rule: "email"},
		validation.FieldError{Field: "/settings/a~1b~0c", Rule: "max", Param: "10", Message: "10 이하여야 합니다"},
		validation.FieldError{Field: "", Rule: "consistent"},
	)
	encoded, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	want := `{"fields":[{"field":"/items/0/email","rule":"email"},{"field":"/settings/a~1b~0c","rule":"max","param":"10","message":"10 이하여야 합니다"},{"field":"","rule":"consistent"}]}`
	if string(encoded) != want {
		t.Fatalf("공통 JSON 형식이 다릅니다:\n받은 값: %s\n기대값: %s", encoded, want)
	}
	var decoded validation.Error
	if unmarshalErr := json.Unmarshal(encoded, &decoded); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	var failure *validation.Error
	if !errors.As(err, &failure) || !reflect.DeepEqual(decoded.Fields, failure.Fields) {
		t.Fatalf("JSON 왕복 후에도 필드별 세부 정보를 유지해야 합니다: %+v", decoded.Fields)
	}
}
