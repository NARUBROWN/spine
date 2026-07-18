package resolver

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/resolver"
)

type DTOResolver struct{}

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
	if err := json.Unmarshal(payload, dtoPtr.Interface()); err != nil {
		return nil, fmt.Errorf("DTO deserialization failed: %w", err)
	}

	return dtoPtr.Elem().Interface(), nil
}
