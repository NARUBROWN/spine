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
		return nil, fmt.Errorf("WebSocketContext가 아닙니다")
	}

	payload := wsCtx.Payload()
	if payload == nil {
		return nil, fmt.Errorf("Payload가 비어있어 DTO를 생성할 수 없습니다")
	}

	dtoPtr := reflect.New(meta.Type)
	if err := json.Unmarshal(payload, dtoPtr.Interface()); err != nil {
		return nil, fmt.Errorf("DTO 역직렬화 실패: %w", err)
	}

	return dtoPtr.Elem().Interface(), nil
}
