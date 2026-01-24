package consumer

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

func (r *DTOResolver) Resolve(ctx core.RequestContext, meta resolver.ParameterMeta) (any, error) {
	consumerCtx, ok := ctx.(core.ConsumerRequestContext)
	if !ok {
		return nil, fmt.Errorf("ConsumerRequestContext가 아닙니다")
	}

	payload := consumerCtx.Payload()
	if payload == nil {
		return nil, fmt.Errorf("Payload가 비어있어 DTO를 생성할 수 없습니다")
	}

	// DTO 인스턴스 생성
	dtoPtr := reflect.New(meta.Type)

	if err := json.Unmarshal(payload, dtoPtr.Interface()); err != nil {
		return nil, fmt.Errorf("DTO 역직렬화에 실패했습니다: %w", err)
	}

	return dtoPtr.Elem().Interface(), nil
}
