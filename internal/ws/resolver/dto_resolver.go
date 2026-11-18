package resolver

import (
	"fmt"
	"reflect"
	"slices"

	"context"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/jsondecode"
	"github.com/NARUBROWN/spine/internal/resolver"
)

type DTOResolver struct {
	Validator validation.Validator
}

func (r *DTOResolver) Supports(meta resolver.ParameterMeta) bool {
	return meta.Type.Kind() == reflect.Struct
}

func (r *DTOResolver) Resolve(ctx context.Context, meta resolver.ParameterMeta, source *Input) (any, error) {
	message := source
	if message == nil {
		return nil, fmt.Errorf("WebSocket message input is not available")
	}

	payload := message.Payload
	if payload == nil {
		return nil, fmt.Errorf("cannot create DTO because payload is empty")
	}

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
