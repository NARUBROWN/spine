package spine

import (
	"context"
	"errors"
	"testing"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type preflightValidator struct{}

func (*preflightValidator) Validate(context.Context, any) error {
	panic("preflight must not validate a DTO")
}

func TestAppValidatorRegistrationPreflight(t *testing.T) {
	for _, application := range []App{New(), New(WithValidator(&preflightValidator{}))} {
		if err := application.Validate(boot.Options{}); err != nil {
			t.Fatalf("검증기를 생략하거나 유효한 구현을 등록한 앱은 사전 검사를 통과해야 합니다: %v", err)
		}
	}
	for _, validator := range []validation.Validator{nil, (*preflightValidator)(nil)} {
		application := New(WithValidator(validator))
		if err := application.Validate(boot.Options{}); !errors.Is(err, validation.ErrInvalidValidator) {
			t.Fatalf("명시적으로 등록한 nil을 거부해야 합니다: %v", err)
		}
		prepared, err := application.Prepare(context.Background(), boot.Options{})
		if prepared != nil || !errors.Is(err, validation.ErrInvalidValidator) {
			t.Fatalf("준비 과정도 같은 오류로 실패해야 합니다: %v", err)
		}
		if err := application.RunContext(context.Background(), boot.Options{}); !errors.Is(err, validation.ErrInvalidValidator) {
			t.Fatalf("관리 실행도 초기화 전에 실패해야 합니다: %v", err)
		}
	}
}
