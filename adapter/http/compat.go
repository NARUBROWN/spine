// Package httpadapter는 이전 HTTP 계약 import 경로의 호환성을 유지합니다.
// 새 구현은 github.com/NARUBROWN/spine/contract/http를 사용합니다.
//
// Deprecated: 공개 계약은 contract/http, contract/event, contract/validator를 사용하세요.
package httpadapter

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
	ServerOptionSupport    = httpadapter.ServerOptionSupport
	NetHTTPHandlerProvider = httpadapter.NetHTTPHandlerProvider
	NetHTTPUpgradeFactory  = httpadapter.NetHTTPUpgradeFactory
	NetHTTPUpgradeRoute    = httpadapter.NetHTTPUpgradeRoute
)

var (
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
