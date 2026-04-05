package boot

import (
	"time"
)

/*
애플리케이션 부트스트랩 전반을 제어하는 최상위 옵션입니다.
서버 실행 방식과 외부 인프라(Kafka, RabbitMQ) 활성화를 결정합니다.
*/
type Options struct {
	// 서버가 바인딩될 주소 (예: ":8080")
	Address string

	// Graceful Shutdown 활성화 여부
	EnableGracefulShutdown bool

	// Graceful Shutdown 시 최대 대기 시간
	ShutdownTimeout time.Duration

	/*
		Kafka 이벤트 인프라 설정입니다.
		nil인 경우 Kafka Producer / Consumer는 구성되지 않습니다.
	*/
	Kafka *KafkaOptions

	/*
			RabbitMQ 이벤트 인프라 설정입니다.
		   	nil인 경우 RabbitMQ 기반 이벤트 처리는 비활성화됩니다.
	*/
	RabbitMQ *RabbitMqOptions

	/*
		HTTP Runtime 전용 설정입니다.
		nil인 경우 HTTP 서버는 실행되지 않습니다.
	*/
	HTTP *HTTPOptions
}

/*
HTTP Runtime 설정입니다.
HTTP 요청 실행 흐름에만 영향을 줍니다.
*/
type HTTPOptions struct {
	// HTTP API 전역 Prefix (예: "/api/v1")
	// 빈 값이면 Prefix를 적용하지 않습니다.
	GlobalPrefix string

	// Recover 미들웨어 비활성화 여부 (기본: false = 활성화)
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

	// keep-alive idle 최대 대기 시간입니다.
	// 0이면 Spine 기본값을 사용합니다.
	IdleTimeout time.Duration

	// HTTP 헤더 최대 크기입니다.
	// 0이면 Spine 기본값을 사용합니다.
	MaxHeaderBytes int

	// HTTP 요청 바디 최대 크기입니다.
	// 0이면 Spine 기본값을 사용하고, 음수면 제한을 비활성화합니다.
	MaxBodyBytes int64

	// WebSocket Runtime 설정입니다.
	WebSocket WebSocketOptions
}

/*
WebSocket Runtime 설정입니다.
*/
type WebSocketOptions struct {
	// 허용할 Origin 목록입니다.
	// 비어 있으면 브라우저 요청에 대해 same-origin만 허용합니다.
	AllowedOrigins []string

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
