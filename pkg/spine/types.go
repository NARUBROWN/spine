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

// Ctx는 HTTP, 이벤트와 WebSocket 핸들러 및 인터셉터의 공통 진입점입니다.
// HTTP와 WebSocket은 현재 실행에서 제공하는 뷰와 지원 여부를 반환합니다.
// Get은 폐기 예정이며 ExecutionContext().Get으로 전환하세요.
type Ctx interface {
	ExecutionContext() ExecutionContextView
	HTTP() (HTTPView, bool)
	WebSocket() (WebSocketView, bool)
	// Deprecated: ExecutionContext().Get을 사용하세요.
	Get(key string) (any, bool)
}

// HTTPView는 HTTP 요청 조회와 현재 응답 writer를 제공합니다.
type HTTPView = contextview.HTTPView

// HTTPRequestView는 HTTP 요청과 동적 멀티파트 필드를 조회합니다.
type HTTPRequestView = contextview.HTTPRequestView

// MultipartFields는 현재 요청의 폼 필드와 파일 헤더 사본입니다.
// 파일은 현재 요청이 완료될 때까지 열 수 있습니다.
type MultipartFields = contextview.MultipartFields

// WebSocketView는 handshake 요청과 현재 메시지 정보를 조회합니다.
// upgrade 전에는 ConnID가 빈 문자열이고 MessageType은 0, Payload는 nil입니다.
// Payload는 독립 사본을 반환합니다.
type WebSocketView = contextview.WebSocketView

// WebSocketRequestView는 upgrade 요청에서 보존한 불변 요청 정보입니다.
type WebSocketRequestView = contextview.WebSocketRequestView
