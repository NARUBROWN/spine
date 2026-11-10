package spine

import (
	"errors"

	"github.com/NARUBROWN/spine/contract/description"
	"github.com/NARUBROWN/spine/internal/execution"
)

// RegistrationDescriber는 앱의 이벤트와 WebSocket 등록을 설명하는 선택적 기능입니다.
type RegistrationDescriber interface {
	DescribeRegistrations() description.Registrations
}

// ErrRegistrationDescriptionUnavailable는 등록 설명 기능이 없는 앱을 나타냅니다.
var ErrRegistrationDescriptionUnavailable = errors.New("application does not provide registration descriptions")

// DescribeRegistrations는 생성자나 전송을 시작하지 않고 현재 등록의 복사본을 반환합니다.
func DescribeRegistrations(application App) (description.Registrations, error) {
	if describer, ok := application.(RegistrationDescriber); ok {
		return describer.DescribeRegistrations(), nil
	}
	return description.Registrations{}, ErrRegistrationDescriptionUnavailable
}

func (a *app) DescribeRegistrations() description.Registrations {
	consumers := a.consumerRegistry.Registrations()
	sockets := a.websocketRegistry.Registrations()
	result := description.Registrations{
		Consumers:  make([]description.ConsumerRegistration, len(consumers)),
		WebSockets: make([]description.WebSocketRegistration, len(sockets)),
	}
	for i, item := range consumers {
		result.Consumers[i] = description.ConsumerRegistration{Topic: item.Topic, Handler: describeRegistrationHandler(item.Meta)}
	}
	for i, item := range sockets {
		result.WebSockets[i] = description.WebSocketRegistration{Path: item.Path, Handler: describeRegistrationHandler(item.Meta)}
	}
	return result
}

func describeRegistrationHandler(meta execution.HandlerMeta) description.Handler {
	receiver := meta.ControllerType.Elem()
	return description.Handler{PackagePath: receiver.PkgPath(), Receiver: receiver.Name(), Method: meta.Method.Name}
}
