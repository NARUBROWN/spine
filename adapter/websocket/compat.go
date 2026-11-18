// Package wsadapter provides WebSocket contract compatibility aliases under adapter.
//
// Deprecated: Use github.com/NARUBROWN/spine/contract/websocket for new implementations.
package wsadapter

import contract "github.com/NARUBROWN/spine/contract/websocket"

const (
	ContractVersion        = contract.ContractVersion
	CloseNormalClosure     = contract.CloseNormalClosure
	ClosePolicyViolation   = contract.ClosePolicyViolation
	CloseInternalServerErr = contract.CloseInternalServerErr
)

type (
	Capabilities = contract.Capabilities
	Options      = contract.Options
	Factory      = contract.Factory
	Adapter      = contract.Adapter
	Connection   = contract.Connection
)

var ErrInvalidAdapter = contract.ErrInvalidAdapter

func ValidateFactory(factory Factory) error          { return contract.ValidateFactory(factory) }
func ValidateAdapter(adapter Adapter) error          { return contract.ValidateAdapter(adapter) }
func ValidateConnection(connection Connection) error { return contract.ValidateConnection(connection) }
