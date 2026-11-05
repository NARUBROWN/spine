// Package httpcontract는 초기 v3 검증 경로를 정식 HTTP 계약에 연결합니다.
// Deprecated: 새 코드는 contract/http를 사용하세요.
package httpcontract

import contract "github.com/NARUBROWN/spine/contract/http"

const ContractVersion = contract.ContractVersion

type (
	HTTPContext            = contract.HTTPContext
	RawQueryContext        = contract.RawQueryContext
	ResponseWriter         = contract.ResponseWriter
	ExecuteFunc            = contract.ExecuteFunc
	Config                 = contract.Config
	Factory                = contract.Factory
	HTTPAdapter            = contract.HTTPAdapter
	Capabilities           = contract.Capabilities
	Requirements           = contract.Requirements
	RequestError           = contract.RequestError
	ServerOptions          = contract.ServerOptions
	ServerOptionSupport    = contract.ServerOptionSupport
	NetHTTPHandlerProvider = contract.NetHTTPHandlerProvider
	NetHTTPUpgradeFactory  = contract.NetHTTPUpgradeFactory
	NetHTTPUpgradeRoute    = contract.NetHTTPUpgradeRoute
)

var (
	ErrInvalidAdapter            = contract.ErrInvalidAdapter
	ErrInvalidState              = contract.ErrInvalidState
	ErrUnsupportedCapability     = contract.ErrUnsupportedCapability
	ErrUnsupportedOption         = contract.ErrUnsupportedOption
	ErrServerClosed              = contract.ErrServerClosed
	ValidateFactory              = contract.ValidateFactory
	ValidateAdapter              = contract.ValidateAdapter
	ValidateNetHTTPUpgradeRoutes = contract.ValidateNetHTTPUpgradeRoutes
	NetHTTPHandler               = contract.NetHTTPHandler
)
