// Package spine는 핸들러와 인터셉터가 현재 실행을 조회하는 공개 컨텍스트 계약입니다.
package spine

import (
	"context"

	"github.com/NARUBROWN/spine/internal/contextview"
)

// ContextCarrier는 현재 실행에 연결된 표준 Go 컨텍스트를 제공합니다.
type ContextCarrier interface {
	Context() context.Context
}

// ExecutionContextView는 현재 실행의 표준 컨텍스트와 공유 값 저장소입니다.
// 저장값은 완료 훅까지 유지하고 실행 종료 시 참조를 해제합니다.
type ExecutionContextView = interface {
	ContextCarrier
	Get(key string) (any, bool)
	Set(key string, value any)
}

// Ctx는 HTTP 핸들러와 인터셉터의 실행 컨텍스트입니다.
// 전송별 조회는 현재 실행에서 제공하는 뷰와 지원 여부를 반환합니다.
// Get은 폐기 예정이며 ExecutionContext().Get으로 전환하세요.
type Ctx interface {
	ExecutionContext() ExecutionContextView
	HTTPRequestContext() (HTTPRequestView, bool)
	HTTPResponseContext() (HTTPResponseView, bool)
	// Deprecated: ExecutionContext().Get을 사용하세요.
	Get(key string) (any, bool)
}

// WebSocketCtx는 프레임의 취소 신호와 실행 중 공유하는 값을 제공합니다.
// 런타임은 완료 훅이 끝나면 저장값을 비우고 프레임을 취소합니다.
type WebSocketCtx interface {
	context.Context
	Set(key string, value any)
	Get(key string) (any, bool)
}

// EventCtx는 이벤트 배달 한 건의 취소와 공유 값을 제공합니다.
// 소비 런타임은 핸들러 실행이 끝나면 값을 비우고 컨텍스트를 취소합니다.
type EventCtx interface {
	context.Context
	Set(key string, value any)
	Get(key string) (any, bool)
}

// HTTPResponseView는 호출할 때마다 현재 응답 writer를 참조하는 공개 뷰입니다.
// 응답 writer 교체와 실행 수명 제어는 노출하지 않습니다.
type HTTPResponseView = contextview.HTTPResponseView

// HTTPRequestView는 HTTP 요청과 동적 멀티파트 필드를 조회합니다.
// 표준 Go 컨텍스트는 Ctx.ExecutionContext().Context()로 조회합니다.
type HTTPRequestView = contextview.HTTPRequestView

// MultipartFields는 현재 요청의 폼 필드와 파일 헤더 사본입니다.
// 파일은 현재 요청이 완료될 때까지 열 수 있습니다.
type MultipartFields = contextview.MultipartFields
