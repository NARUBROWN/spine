// Package wsadapter는 adapter 아래에서 WebSocket 계약의 호환 별칭을 제공합니다.
// Deprecated: 새 구현은 github.com/NARUBROWN/spine/contract/websocket을 사용하세요.
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
