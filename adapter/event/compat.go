// Package eventadapter는 이전 이벤트 계약 경로의 호환 별칭입니다.
// Deprecated: 새 구현은 github.com/NARUBROWN/spine/contract/event를 사용하세요.
package eventadapter

import eventcontract "github.com/NARUBROWN/spine/contract/event"

const ContractVersion = eventcontract.ContractVersion

type (
	Message      = eventcontract.Message
	Subscription = eventcontract.Subscription
	Delivery     = eventcontract.Delivery
	Reader       = eventcontract.Reader
	Publisher    = eventcontract.Publisher
	Factory      = eventcontract.Factory
	Capabilities = eventcontract.Capabilities
	Requirements = eventcontract.Requirements
)

var (
	ErrInvalidAdapter        = eventcontract.ErrInvalidAdapter
	ErrUnsupportedCapability = eventcontract.ErrUnsupportedCapability
	ErrInvalidState          = eventcontract.ErrInvalidState
	ErrReaderInvalidated     = eventcontract.ErrReaderInvalidated
)

func ValidateFactory(factory Factory, required Requirements) error {
	return eventcontract.ValidateFactory(factory, required)
}
