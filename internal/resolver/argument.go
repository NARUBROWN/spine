package resolver

import (
	"reflect"

	"github.com/NARUBROWN/spine/internal/execution"
)

type ParameterMeta struct {
	Index   int
	Type    reflect.Type
	PathKey string
}

/*
ArgumentResolver는 핸들러 메서드의 매개변수 값을
하나의 실행 컨텍스트로부터 생성하는 계약입니다.

각 리졸버는 자신이 처리할 수 있는 타입인지 확인하고,
해당 타입에 맞는 값을 실행 컨텍스트를 통해 조회합니다.
*/
type ArgumentResolver interface {
	// Supports는 해당 매개변수 타입을 이 리졸버가 처리할 수 있는지 반환합니다.
	Supports(parameterMeta ParameterMeta) bool

	// Resolve는 실행 컨텍스트를 받아 핸들러 인자를 생성합니다.
	Resolve(ctx *execution.Context, parameterMeta ParameterMeta) (any, error)
}
