package resolver

import (
	"context"
	"errors"
	"reflect"
	"testing"

	validation "github.com/NARUBROWN/spine/contract/validator"
)

type validatorFunc func(context.Context, any) error

func (f validatorFunc) Validate(ctx context.Context, dto any) error { return f(ctx, dto) }

func TestHTTPDTOValidationRunsAfterBinding(t *testing.T) {
	type bodyDTO struct {
		Name string `json:"name"`
	}
	type formDTO struct {
		Name string `form:"name"`
	}
	for _, isForm := range []bool{false, true} {
		for _, reject := range []bool{false, true} {
			ctx := newFakeHttpCtx()
			ctx.bindValue = bodyDTO{Name: "Kim"}
			pm := ParameterMeta{Type: reflect.TypeFor[*bodyDTO]()}
			failure := validation.NewError(validation.FieldError{Field: "/name", Rule: "custom"})
			calls := 0
			validator := validatorFunc(func(_ context.Context, dto any) error {
				calls++
				if reflect.ValueOf(dto).Elem().FieldByName("Name").String() != "Kim" {
					t.Fatal("검증기는 바인딩한 DTO를 받아야 합니다")
				}
				if reject {
					return failure
				}
				return nil
			})
			var r ArgumentResolver = &DTOResolver{Validator: validator}
			if isForm {
				ctx.bindValue = formDTO{Name: "Kim"}
				pm.Type = reflect.TypeFor[*formDTO]()
				r = &FormDTOResolver{Validator: validator}
			}
			value, err := r.Resolve(ctx, pm)
			if calls != 1 || (reject && (value != nil || !errors.Is(err, failure))) || (!reject && (err != nil || value == nil)) {
				t.Fatalf("폼=%v 거부=%v: 호출=%d 값=%v 오류=%v", isForm, reject, calls, value, err)
			}
			ctx.bindErr = errors.New("binding failed")
			if _, err := r.Resolve(ctx, pm); !errors.Is(err, ctx.bindErr) || calls != 1 {
				t.Fatalf("바인딩 실패 뒤 검증기를 호출하면 안 됩니다: %v", err)
			}
		}
	}
}

func TestValidateDTOPreservesErrorsAndRejectsInvalidFailures(t *testing.T) {
	failure := validation.NewError(validation.FieldError{Field: "/name", Rule: "required"})
	processingErr := errors.New("validator processing failed")
	for _, tt := range []struct{ err, want error }{
		{failure, failure}, {processingErr, processingErr},
		{&validation.Error{}, validation.ErrInvalidValidator},
		{(*validation.Error)(nil), validation.ErrInvalidValidator},
	} {
		err := ValidateDTO(context.Background(), validatorFunc(func(context.Context, any) error { return tt.err }), struct{}{})
		if !errors.Is(err, tt.want) {
			t.Fatalf("오류 구분과 원인을 보존해야 합니다: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ValidateDTO(ctx, validatorFunc(func(context.Context, any) error {
		t.Fatal("취소된 요청에서 검증하면 안 됩니다")
		return nil
	}), struct{}{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("시작 전 취소를 보존해야 합니다: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	err := ValidateDTO(ctx, validatorFunc(func(context.Context, any) error { cancel(); return processingErr }), struct{}{})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, processingErr) {
		t.Fatalf("검증 중 취소와 기존 오류를 함께 보존해야 합니다: %v", err)
	}
}
