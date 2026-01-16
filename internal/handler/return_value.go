package handler

import (
	"reflect"

	"github.com/NARUBROWN/spine/core"
)

/*
ReturnValueHandler는 Handler 메서드의 반환값을 외부 응답으로 변환하는 계약입니다.
반환값의 타입에 따라 적절한 방식으로 응답을 처리하는 책임을 가집니다.
*/
type ReturnValueHandler interface {
	// Supports는 해당 반환 타입을 이 Handler가 처리할 수 있는지 여부를 판단합니다.
	Supports(returnType reflect.Type) bool

	// Handle은 Handler의 반환값을 실제 응답으로 처리합니다.
	Handle(value any, ctx core.ExecutionContext) error
}
