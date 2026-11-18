// Package eventadapter provides compatibility aliases for the legacy event contract path.
//
// Deprecated: Use github.com/NARUBROWN/spine/contract/event for new implementations.
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
