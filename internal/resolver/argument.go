package resolver

import (
	"reflect"

	"github.com/NARUBROWN/spine/core"
)

type ParameterMeta struct {
	Index int
	Type  reflect.Type
}

/*
ArgumentResolver는 Handler 메서드의 파라미터 값을
실행 시점에 Context로부터 생성하는 계약입니다.

각 Resolver는 자신이 처리할 수 있는 타입인지 확인하고,
해당 타입에 맞는 값을 Context로부터 추출합니다.
*/
type ArgumentResolver interface {
	// Supports는 해당 파라미터 타입을 이 Resolver가 처리할 수 있는지 여부를 반환합니다.
	Supports(parameterMeta ParameterMeta) bool

	// Resolve는 Context를 기반으로 파라미터에 전달될 값을 생성합니다.
	Resolve(ctx core.Context, parameterMeta ParameterMeta) (any, error)
}
