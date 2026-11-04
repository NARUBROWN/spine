package boot

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// InterceptorScope는 전역 인터셉터를 실행할 기본 전송 방식을 선택합니다.
type InterceptorScope uint8

const (
	// InterceptorHTTP는 HTTP 요청에만 인터셉터를 실행합니다.
	InterceptorHTTP InterceptorScope = 1 << iota
	// InterceptorWebSocket은 WebSocket 메시지에만 인터셉터를 실행합니다.
	InterceptorWebSocket
	// InterceptorAll은 HTTP 요청과 WebSocket 메시지에 인터셉터를 실행합니다.
	InterceptorAll = InterceptorHTTP | InterceptorWebSocket

	// DefaultWebSocketMaxConnections는 Spine의 기본 동시 연결 수 제한입니다.
	DefaultWebSocketMaxConnections = 1024
	// UnlimitedWebSocketConnections는 동시 연결 수 제한을 명시적으로 비활성화합니다.
	UnlimitedWebSocketConnections = -1
)

/*
애플리케이션 부트스트랩 전반을 제어하는 최상위 옵션입니다.
HTTP 서버 실행과 공통 종료 정책을 설정합니다. 이벤트 플랫폼은 App 생성 옵션으로 등록합니다.
*/
type Options struct {
	// 서버가 바인딩될 주소 (예: ":8080")
	Address string

	// 정상 종료 활성화 여부
	EnableGracefulShutdown bool

	// 정상 종료 시 최대 대기 시간
	ShutdownTimeout time.Duration

	/*
		HTTP 런타임 전용 설정입니다.
		nil인 경우 HTTP 서버는 실행되지 않습니다.
	*/
	HTTP *HTTPOptions
}

/*
HTTP 런타임 설정입니다.
HTTP 요청 실행 흐름에만 영향을 줍니다.
*/
type HTTPOptions struct {
	// OpenAPI는 Enabled가 true일 때 별도 CLI나 주석 없이 제공하는 표준 HTTP 명세의 설정입니다.
	OpenAPI OpenAPIOptions

	// HTTP API 전역 접두사(예: "/api/v1")
	// 빈 값이면 접두사를 적용하지 않습니다.
	GlobalPrefix string

	// 복구 미들웨어 비활성화 여부(기본: false = 활성화)
	DisableRecover bool

	// HTTP 헤더 수신 최대 대기 시간입니다.
	// 0이면 Spine 기본값을 사용합니다.
	ReadHeaderTimeout time.Duration

	// HTTP 요청 전체 읽기 최대 대기 시간입니다.
	// 0이면 Spine 기본값을 사용합니다.
	ReadTimeout time.Duration

	// HTTP 응답 쓰기 최대 대기 시간입니다.
	// 0이면 Spine 기본값을 사용합니다.
	WriteTimeout time.Duration

	// 연결 유지 유휴 상태의 최대 대기 시간입니다.
	// 0이면 Spine 기본값을 사용합니다.
	IdleTimeout time.Duration

	// HTTP 헤더 최대 크기입니다.
	// 0이면 Spine 기본값을 사용합니다.
	MaxHeaderBytes int

	// HTTP 요청 바디 최대 크기입니다.
	// 0이면 Spine 기본값을 사용하고, 음수면 제한을 비활성화합니다.
	MaxBodyBytes int64

	// WebSocket 런타임 설정입니다.
	WebSocket WebSocketOptions
}

// ValidateIssues는 네트워크에 연결하지 않고 HTTP와 WebSocket 설정을 검증합니다.
func (o HTTPOptions) ValidateIssues(prefix string) []ConfigIssue {
	if prefix == "" {
		prefix = "HTTP"
	}
	issues := o.OpenAPI.ValidateIssues(configPath(prefix, "OpenAPI"))
	for _, field := range []struct {
		name  string
		value time.Duration
		code  string
	}{
		{"ReadHeaderTimeout", o.ReadHeaderTimeout, "HTTP_READ_HEADER_TIMEOUT_INVALID"},
		{"ReadTimeout", o.ReadTimeout, "HTTP_READ_TIMEOUT_INVALID"},
		{"WriteTimeout", o.WriteTimeout, "HTTP_WRITE_TIMEOUT_INVALID"},
		{"IdleTimeout", o.IdleTimeout, "HTTP_IDLE_TIMEOUT_INVALID"},
	} {
		if field.value < 0 {
			issues = append(issues, ConfigIssue{Path: configPath(prefix, field.name), Code: field.code, Message: "HTTP timeout cannot be negative.", Hint: "Use zero for the secure default or a positive duration."})
		}
	}
	if o.MaxHeaderBytes < 0 {
		issues = append(issues, ConfigIssue{Path: configPath(prefix, "MaxHeaderBytes"), Code: "HTTP_MAX_HEADER_BYTES_INVALID", Message: "HTTP maximum header size cannot be negative.", Hint: "Use zero for the secure default or a positive byte limit."})
	}
	issues = append(issues, o.WebSocket.ValidateIssues(configPath(prefix, "WebSocket"))...)
	return issues
}

func (o HTTPOptions) Validate() error {
	return ValidationError(o.ValidateIssues("HTTP")...)
}

/*
WebSocket 런타임 설정입니다.
*/
type WebSocketOptions struct {
	// MaxConnections는 동시에 추적하는 WebSocket 연결 수를 제한합니다.
	// 0이면 DefaultWebSocketMaxConnections를 사용합니다. 제한을 명시적으로 비활성화하려면
	// UnlimitedWebSocketConnections를 사용합니다.
	MaxConnections int

	// CapacityRetryAfter는 연결 수 제한에 도달했을 때 반환하는 Retry-After 응답을 제어합니다.
	// 0이면 Spine 기본값을 사용합니다.
	CapacityRetryAfter time.Duration

	// 허용할 출처 목록입니다.
	// 비어 있으면 브라우저 요청에 대해 동일 출처만 허용합니다.
	AllowedOrigins []string

	// TrustedProxyCIDRs는 기본 동일 출처 검사에서 Forwarded 또는 X-Forwarded-Proto 헤더로
	// 요청 스킴을 결정하도록 신뢰할 리버스 프록시 목록입니다. 그 외 피어의 전달 헤더는
	// 무시하며, 명시적으로 설정한 AllowedOrigins에는 영향을 주지 않습니다.
	TrustedProxyCIDRs []string

	// 허용할 최대 메시지 크기입니다.
	// 0이면 Spine 기본값을 사용합니다.
	MaxMessageBytes int64

	// 핸드셰이크 최대 대기 시간입니다.
	// 0이면 Spine 기본값을 사용합니다.
	HandshakeTimeout time.Duration

	// 메시지 수신 또는 pong 대기 최대 시간입니다.
	// 0이면 Spine 기본값을 사용합니다.
	ReadTimeout time.Duration

	// 메시지 또는 제어 프레임 쓰기 최대 시간입니다.
	// 0이면 Spine 기본값을 사용합니다.
	WriteTimeout time.Duration

	// 서버 ping 전송 간격입니다.
	// 0이면 Spine 기본값을 사용합니다.
	PingInterval time.Duration
}

// ValidateIssues는 네트워크에 연결하지 않고 WebSocket 설정을 검증합니다.
func (o WebSocketOptions) ValidateIssues(prefix string) []ConfigIssue {
	if prefix == "" {
		prefix = "HTTP.WebSocket"
	}
	var issues []ConfigIssue
	if o.MaxConnections < UnlimitedWebSocketConnections {
		issues = append(issues, ConfigIssue{Path: configPath(prefix, "MaxConnections"), Code: "WEBSOCKET_MAX_CONNECTIONS_INVALID", Message: "WebSocket connection limit is invalid.", Hint: "Use zero for the default, a positive limit, or boot.UnlimitedWebSocketConnections."})
	}
	if o.CapacityRetryAfter < 0 {
		issues = append(issues, ConfigIssue{Path: configPath(prefix, "CapacityRetryAfter"), Code: "WEBSOCKET_RETRY_AFTER_INVALID", Message: "WebSocket capacity retry duration cannot be negative.", Hint: "Use zero for the default or a positive duration."})
	}
	if o.MaxMessageBytes < 0 {
		issues = append(issues, ConfigIssue{Path: configPath(prefix, "MaxMessageBytes"), Code: "WEBSOCKET_MAX_MESSAGE_BYTES_INVALID", Message: "WebSocket maximum message size cannot be negative.", Hint: "Use zero for the secure default or a positive byte limit."})
	}
	for _, field := range []struct {
		name  string
		value time.Duration
		code  string
	}{
		{"HandshakeTimeout", o.HandshakeTimeout, "WEBSOCKET_HANDSHAKE_TIMEOUT_INVALID"},
		{"ReadTimeout", o.ReadTimeout, "WEBSOCKET_READ_TIMEOUT_INVALID"},
		{"WriteTimeout", o.WriteTimeout, "WEBSOCKET_WRITE_TIMEOUT_INVALID"},
		{"PingInterval", o.PingInterval, "WEBSOCKET_PING_INTERVAL_INVALID"},
	} {
		if field.value < 0 {
			issues = append(issues, ConfigIssue{Path: configPath(prefix, field.name), Code: field.code, Message: "WebSocket timeout cannot be negative.", Hint: "Use zero for the secure default or a positive duration."})
		}
	}
	for i, cidr := range o.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(cidr)); err != nil {
			issues = append(issues, ConfigIssue{Path: fmt.Sprintf("%s.TrustedProxyCIDRs[%d]", prefix, i), Code: "WEBSOCKET_TRUSTED_PROXY_CIDR_INVALID", Message: "Trusted proxy CIDR is invalid.", Hint: "Use CIDR notation such as 10.0.0.0/8 or 2001:db8::/32."})
		}
	}
	for i, origin := range o.AllowedOrigins {
		if origin == "*" {
			continue
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			issues = append(issues, ConfigIssue{Path: fmt.Sprintf("%s.AllowedOrigins[%d]", prefix, i), Code: "WEBSOCKET_ALLOWED_ORIGIN_INVALID", Message: "Allowed WebSocket origin is invalid.", Hint: "Use an exact http:// or https:// origin without a path, query, or fragment."})
		}
	}
	return issues
}

func (o WebSocketOptions) Validate() error {
	return ValidationError(o.ValidateIssues("HTTP.WebSocket")...)
}
