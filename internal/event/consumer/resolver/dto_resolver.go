package resolver

import (
	"fmt"
	"reflect"
	"slices"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/jsondecode"
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/spine"
)

type DTOResolver struct {
	Validator validation.Validator
}

func (r *DTOResolver) Supports(meta resolver.ParameterMeta) bool {
	return meta.Type.Kind() == reflect.Struct
}

func (r *DTOResolver) Resolve(ctx spine.EventCtx, meta resolver.ParameterMeta, message *Input) (any, error) {
	if message == nil {
		return nil, fmt.Errorf("consumer message input is not available")
	}

	payload := message.Payload
	if payload == nil {
		return nil, fmt.Errorf("cannot create DTO because payload is empty")
	}

	// DTO 인스턴스 생성
	dtoPtr := reflect.New(meta.Type)

	if err := jsondecode.Unmarshal(slices.Clone(payload), dtoPtr.Interface()); err != nil {
		return nil, fmt.Errorf("DTO deserialization failed: %w", err)
	}

	dto := dtoPtr.Elem().Interface()
	if err := resolver.ValidateDTO(ctx, r.Validator, dto); err != nil {
		return nil, err
	}
	return dto, nil
}
