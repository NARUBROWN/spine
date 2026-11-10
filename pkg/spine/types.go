package spine

import "context"

// ContextCarrier는 현재 실행에 연결된 표준 Go 컨텍스트를 제공합니다.
type ContextCarrier interface {
	Context() context.Context
}

// ExecutionContextView는 현재 실행의 표준 컨텍스트와 값 저장소를 읽는 인터페이스입니다.
type ExecutionContextView = interface {
	ContextCarrier
	Get(key string) (any, bool)
}

// Ctx는 핸들러에 주입되는 실행 컨텍스트입니다.
type Ctx interface {
	ExecutionContext() ExecutionContextView

	// Deprecated: ExecutionContext().Get을 사용하세요.
	Get(key string) (any, bool)
}
