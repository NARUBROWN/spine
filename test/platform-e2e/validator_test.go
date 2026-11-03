package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	playground "github.com/NARUBROWN/spine-validator-playground"
	validation "github.com/NARUBROWN/spine/contract/validator"
)

func newRealValidator(t *testing.T) validation.Validator {
	t.Helper()
	v, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestFinalValidatorContract(t *testing.T) {
	v := newRealValidator(t)
	ctx := context.Background()
	valid := validatedInput{Email: "kim@example.com", Age: 20}
	for _, value := range []any{valid, &valid} {
		if err := v.Validate(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	var nilInput *validatedInput
	for _, value := range []any{nil, nilInput, "private-invalid", 123, []validatedInput{}} {
		if err := v.Validate(ctx, value); !errors.Is(err, validation.ErrInvalidValue) {
			t.Fatalf("입력 타입 %T: %v", value, err)
		}
	}
	type nested struct {
		Members map[string][]validatedInput `json:"members" validate:"dive,dive"`
	}
	input := nested{Members: map[string][]validatedInput{
		"a/b~c[.x]": {{Email: "private-invalid", Age: 17}},
		"a]x[.b":    {{Email: "private-invalid", Age: 17}},
	}}
	before, _ := json.Marshal(input)
	var expected []validation.FieldError
	for range 20 {
		err := v.Validate(ctx, input)
		failure, ok := validation.AsError(err)
		if !ok || len(failure.Fields) != 4 {
			t.Fatalf("중첩 필드 오류: %v", err)
		}
		if expected == nil {
			expected = append([]validation.FieldError(nil), failure.Fields...)
		} else if !reflect.DeepEqual(expected, failure.Fields) {
			t.Fatal("맵 오류 순서가 달라졌습니다")
		}
		data, _ := json.Marshal(failure)
		if strings.Contains(string(data), "private-invalid") || strings.Contains(err.Error(), "private-invalid") {
			t.Fatal("거부된 값을 오류에 노출하면 안 됩니다")
		}
	}
	if expected[0].Field != "/members/a]x[.b/0/age" || expected[2].Field != "/members/a~1b~0c[.x]/0/age" {
		t.Fatalf("JSON Pointer 이스케이프: %+v", expected)
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("입력을 변경하면 안 됩니다")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := v.Validate(canceled, valid); !errors.Is(err, context.Canceled) {
		t.Fatalf("취소 원인: %v", err)
	}
}
