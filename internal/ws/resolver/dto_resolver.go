package resolver

import (
	"fmt"
	"reflect"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/jsonx"
)

type DTOResolver struct {
	Validator validation.Validator
}

func (r *DTOResolver) Supports(meta resolver.ParameterMeta) bool {
	return meta.Type.Kind() == reflect.Struct
}

func (r *DTOResolver) Resolve(ctx core.ExecutionContext, meta resolver.ParameterMeta) (any, error) {
	wsCtx, ok := ctx.(core.WebSocketContext)
	if !ok {
		return nil, fmt.Errorf("context is not a WebSocketContext")
	}

	payload := wsCtx.Payload()
	if payload == nil {
		return nil, fmt.Errorf("cannot create DTO because payload is empty")
	}

	dtoPtr := reflect.New(meta.Type)
	if err := jsonx.Unmarshal(payload, dtoPtr.Interface()); err != nil {
		return nil, fmt.Errorf("DTO deserialization failed: %w", err)
	}

	dto := dtoPtr.Elem().Interface()
	if err := resolver.ValidateDTO(ctx.Context(), r.Validator, dto); err != nil {
		return nil, err
	}
	return dto, nil
}
