package resolver

import (
	"context"
	"errors"
	"fmt"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/pkg/httperr"
)

// ValidateDTO는 바인딩을 마친 DTO에만 등록된 검증기를 호출합니다.
// 컨텍스트와 원래 오류를 보존하여 모든 전송 방식이 같은 결과를 받도록 합니다.
func ValidateDTO(ctx context.Context, validator validation.Validator, dto any) error {
	if validator == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err := validator.Validate(ctx, dto)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return errors.Join(ctxErr, err)
	}
	if err == nil {
		return nil
	}
	var failure *validation.Error
	if errors.As(err, &failure) && (failure == nil || len(failure.Fields) == 0) {
		return fmt.Errorf("%w: validation failure has no field errors", validation.ErrInvalidValidator)
	}
	return fmt.Errorf("DTO validation failed: %w", err)
}

// ValidateHTTPDTO는 바인딩 출처의 이름으로 검증하고 HTTP 오류에만 출처를 덧붙입니다.
func ValidateHTTPDTO(ctx context.Context, validator validation.Validator, dto any, source, tag string) error {
	if validator == nil {
		return nil
	}
	err := ValidateDTO(validation.WithFieldTags(ctx, tag), validator, dto)
	if _, ok := validation.AsError(err); ok {
		return &httperr.InputError{Source: source, Cause: err}
	}
	return err
}
