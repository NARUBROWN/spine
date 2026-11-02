package eventadapter

import "errors"

var (
	// ErrInvalidAdapter는 nil 객체, 잘못된 구성 또는 계약 버전 불일치를 나타냅니다.
	ErrInvalidAdapter = errors.New("invalid event adapter")
	// ErrUnsupportedCapability는 factory가 요청한 전송 역할을 지원하지 않음을 나타냅니다.
	ErrUnsupportedCapability = errors.New("unsupported event adapter capability")
	// ErrInvalidState는 종료된 자원 사용이나 같은 전달의 중복 Ack/Nack을 나타냅니다.
	ErrInvalidState = errors.New("invalid event adapter state")
	// ErrReaderInvalidated는 같은 Reader에서 다음 메시지를 읽기 전에 닫고 다시 생성해야 함을 나타냅니다.
	// Kafka에서 실패한 offset보다 높은 offset이 커밋되는 것을 막는 데 사용할 수 있습니다.
	ErrReaderInvalidated = errors.New("event adapter reader invalidated")
)
