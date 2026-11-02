// Package eventadapter는 Spine 커널과 브로커 구현이 공유하는 공개 이벤트 전송 계약입니다.
// 표준 라이브러리만 사용하며 도메인 이벤트, DI, 파이프라인과 브로커 클라이언트를 참조하지 않습니다.
package eventadapter

import (
	"context"
	"time"
)

const ContractVersion = "spine.event-adapter/v1"

// Message는 직렬화된 이벤트와 전송 메타데이터입니다.
// EventName은 발행할 이벤트의 논리 이름이며 수신 시 다른 핸들러를 선택하는 근거로 쓰지 않습니다.
// Payload의 형식은 커널과 애플리케이션이 정하고, 어댑터는 내용을 해석하거나 재직렬화하지 않습니다.
// Payload와 Metadata는 호출 중 읽기 전용입니다. 참조 데이터의 수명은 Reader와 Publisher 계약을 따릅니다.
type Message struct {
	EventName  string
	Payload    []byte
	OccurredAt time.Time
	Metadata   map[string]string
}

// Subscription은 커널에 등록된 소비 경로 하나를 나타냅니다.
// Topic은 비어 있지 않은 논리 구독 키입니다. native topic, queue, routing key와의 연결은 factory가 소유합니다.
// 커널은 이 구독에 연결한 핸들러를 실행하며 Message.EventName이나 생산자 메타데이터로 재라우팅하지 않습니다.
type Subscription struct {
	Topic string
}

// Delivery는 메시지 한 건과 그 전달의 처리 결과를 브로커에 반영하는 계약입니다.
// 커널은 핸들러와 후처리까지 성공하면 Ack, 실패하면 Nack을 한 번 호출합니다.
// Ack/Nack이 오류를 반환해도 다른 결과로 다시 확인하지 않습니다. 재전달은 중복 실행을 허용합니다.
type Delivery interface {
	Message() Message
	// Ack는 설정된 성공 확인을 마칠 때까지 기다립니다. Kafka에서는 offset commit에 해당합니다.
	Ack(context.Context) error
	// Nack은 factory에 설정된 실패 정책을 적용합니다. RabbitMQ의 reject/requeue 등이 해당합니다.
	// 안전하게 다음 메시지를 읽을 수 없으면 ErrReaderInvalidated로 감싼 오류를 반환합니다.
	Nack(context.Context) error
}

// Reader는 구독 하나의 메시지를 순서대로 전달합니다.
// Read와 Ack/Nack은 직렬로 호출하며 전달 하나의 확인이 끝나기 전에 다음 Read를 호출하지 않습니다.
// 병렬 처리가 필요하면 독립된 Reader를 사용합니다. 순서 보장 범위는 브로커 구현이 명시합니다.
type Reader interface {
	// Read는 메시지나 오류가 생길 때까지 기다리며 ctx 취소에 응답합니다. (nil, nil)은 허용하지 않습니다.
	// Message의 참조 데이터는 Ack/Nack 완료 또는 Close 시작 전까지 유효합니다.
	// 이후에도 보관하려면 호출자가 Payload와 Metadata를 복사해야 합니다.
	Read(context.Context) (Delivery, error)
	// Close는 새 읽기를 막고 대기 중인 Read를 해제한 뒤 연결을 정리합니다.
	// 진행 중인 Ack/Nack과 자원 정리는 한 번만 완료하며 여러 호출이 그 결과를 기다릴 수 있습니다.
	// ctx는 완료 대기만 제한합니다. 시간 초과 뒤에도 정리는 계속되며 새 context로 다시 기다릴 수 있습니다.
	Close(context.Context) error
}

// Publisher는 직렬화된 이벤트를 동기 발행합니다. Publish는 동시에 호출할 수 있습니다.
// 성공은 구현에 명시된 전송 확인 정책의 완료를 뜻하며 소비 완료나 중복 없는 전달을 보장하지 않습니다.
type Publisher interface {
	// Publish는 입력을 변경하지 않으며 반환 후에도 참조하려면 Payload와 Metadata를 복사합니다.
	// 오류나 취소 시 브로커가 이미 수신했을 수 있으므로 재시도는 중복 발행을 일으킬 수 있습니다.
	Publish(context.Context, Message) error
	// Close는 새 발행을 막고 진행 중인 Publish가 끝난 뒤 자원을 한 번 정리합니다.
	// ctx는 완료 대기만 제한하며 후속 Close는 같은 정리 결과를 기다립니다.
	Close(context.Context) error
}

// Factory는 기능을 선언하고 독립된 Reader와 Publisher를 생성합니다.
// Capabilities는 네트워크나 native 훅을 실행하지 않는 불변 선언입니다.
// factory 메서드는 동시에 호출할 수 있으며 브로커별 typed 옵션과 자격 증명은 구현이 소유합니다.
type Factory interface {
	Capabilities() Capabilities
	// NewReader는 연결과 구독 readiness를 확인하되 핸들러 실행이나 Ack/Nack을 시작하지 않습니다.
	// 성공하면 nil이 아닌 Reader의 소유권을 넘깁니다. 실패하면 획득한 자원을 정리하고 nil을 반환합니다.
	NewReader(context.Context, Subscription) (Reader, error)
	// NewPublisher는 발행 자원을 준비합니다. 성공은 개별 발행 대상의 존재나 전달 성공을 보장하지 않습니다.
	// 성공하면 nil이 아닌 Publisher의 소유권을 넘기고 실패하면 자원을 정리하고 nil을 반환합니다.
	NewPublisher(context.Context) (Publisher, error)
}
