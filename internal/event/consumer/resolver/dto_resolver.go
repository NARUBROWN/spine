package resolver

import (
	"fmt"
	"reflect"
	"slices"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/jsondecode"
	"github.com/NARUBROWN/spine/internal/resolver"
)

type DTOResolver struct {
	Validator validation.Validator
}

func (r *DTOResolver) Supports(meta resolver.ParameterMeta) bool {
	return meta.Type.Kind() == reflect.Struct
}

func (r *DTOResolver) Resolve(ctx execution.Context, meta resolver.ParameterMeta) (any, error) {
	consumerCtx, ok := ctx.(execution.ConsumerRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not a ConsumerRequestContext")
	}

	payload := consumerCtx.Payload()
	if payload == nil {
		return nil, fmt.Errorf("cannot create DTO because payload is empty")
	}

	// DTO 인스턴스 생성
	dtoPtr := reflect.New(meta.Type)

	if err := jsondecode.Unmarshal(slices.Clone(payload), dtoPtr.Interface()); err != nil {
		return nil, fmt.Errorf("DTO deserialization failed: %w", err)
	}

	dto := dtoPtr.Elem().Interface()
	if err := resolver.ValidateDTO(ctx.Context(), r.Validator, dto); err != nil {
		return nil, err
	}
	return dto, nil
}
