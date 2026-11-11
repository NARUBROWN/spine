package boot

import "github.com/NARUBROWN/spine/internal/configuration"

// Options는 앱 실행과 공통 종료 정책의 공개 설정입니다. 검증은 app.Validate를 사용합니다.
type Options = configuration.Options

// HTTPOptions는 HTTP 실행 설정입니다. nil인 경우 HTTP 서버를 실행하지 않습니다.
type HTTPOptions = configuration.HTTPOptions

// WebSocketOptions는 WebSocket 연결과 메시지 실행의 공개 설정입니다.
type WebSocketOptions = configuration.WebSocketOptions

// InterceptorScope는 전역 인터셉터를 실행할 전송 범위입니다.
type InterceptorScope = configuration.InterceptorScope

const (
	InterceptorHTTP                = configuration.InterceptorHTTP
	InterceptorWebSocket           = configuration.InterceptorWebSocket
	InterceptorAll                 = configuration.InterceptorAll
	DefaultWebSocketMaxConnections = configuration.DefaultWebSocketMaxConnections
	UnlimitedWebSocketConnections  = configuration.UnlimitedWebSocketConnections
)
