// Package adapter는 기존 HTTP 계약 import 경로의 호환성을 유지합니다.
// 새 구현은 github.com/NARUBROWN/spine/adapter/http를 사용합니다.
//
// Deprecated: HTTP 계약은 adapter/http, 이벤트 계약은 adapter/event를 사용하세요.
package adapter

import httpadapter "github.com/NARUBROWN/spine/adapter/http"

const ContractVersion = httpadapter.ContractVersion

type (
	HTTPContext    = httpadapter.HTTPContext
	ResponseWriter = httpadapter.ResponseWriter
	ExecuteFunc    = httpadapter.ExecuteFunc
	UpgradeRoute   = httpadapter.UpgradeRoute
	Config         = httpadapter.Config
	Factory        = httpadapter.Factory
	HTTPAdapter    = httpadapter.HTTPAdapter
	Capabilities   = httpadapter.Capabilities
	Requirements   = httpadapter.Requirements
	RequestError   = httpadapter.RequestError
	ServerOptions  = httpadapter.ServerOptions
)

var (
	ErrInvalidAdapter        = httpadapter.ErrInvalidAdapter
	ErrInvalidState          = httpadapter.ErrInvalidState
	ErrUnsupportedCapability = httpadapter.ErrUnsupportedCapability
)

// ValidateFactory는 기존 호출을 같은 HTTP 계약 검증으로 전달합니다.
func ValidateFactory(factory Factory, required Requirements) error {
	return httpadapter.ValidateFactory(factory, required)
}
