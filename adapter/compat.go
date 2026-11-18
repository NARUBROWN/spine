// Package adapter preserves compatibility with the legacy HTTP contract import path.
// New implementations use github.com/NARUBROWN/spine/contract/http.
//
// Deprecated: Use github.com/NARUBROWN/spine/contract/http for new HTTP implementations.
package adapter

import httpadapter "github.com/NARUBROWN/spine/contract/http"

const ContractVersion = httpadapter.ContractVersion

type (
	HTTPContext            = httpadapter.HTTPContext
	RawQueryContext        = httpadapter.RawQueryContext
	ResponseWriter         = httpadapter.ResponseWriter
	ExecuteFunc            = httpadapter.ExecuteFunc
	Config                 = httpadapter.Config
	Factory                = httpadapter.Factory
	HTTPAdapter            = httpadapter.HTTPAdapter
	Capabilities           = httpadapter.Capabilities
	Requirements           = httpadapter.Requirements
	RequestError           = httpadapter.RequestError
	ServerOptions          = httpadapter.ServerOptions
	ServeOptions           = httpadapter.ServeOptions
	ServeOption            = httpadapter.ServeOption
	ServerOptionSupport    = httpadapter.ServerOptionSupport
	NetHTTPHandlerProvider = httpadapter.NetHTTPHandlerProvider
	NetHTTPUpgradeFactory  = httpadapter.NetHTTPUpgradeFactory
	NetHTTPUpgradeRoute    = httpadapter.NetHTTPUpgradeRoute
)

var (
	WithTLS                      = httpadapter.WithTLS
	ErrUnsupportedOption         = httpadapter.ErrUnsupportedOption
	ErrServerClosed              = httpadapter.ErrServerClosed
	ValidateAdapter              = httpadapter.ValidateAdapter
	ValidateNetHTTPUpgradeRoutes = httpadapter.ValidateNetHTTPUpgradeRoutes
	NetHTTPHandler               = httpadapter.NetHTTPHandler
	ErrInvalidAdapter            = httpadapter.ErrInvalidAdapter
	ErrInvalidState              = httpadapter.ErrInvalidState
	ErrUnsupportedCapability     = httpadapter.ErrUnsupportedCapability
)

// ValidateFactory는 기존 호출을 같은 HTTP 계약 검증으로 전달합니다.
func ValidateFactory(factory Factory, required Requirements) error {
	return httpadapter.ValidateFactory(factory, required)
}
