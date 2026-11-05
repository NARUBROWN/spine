package httpcontract

import (
	"fmt"
	"net/http"
)

// NetHTTPHandlerProvider는 net/http 서버에 연결할 수 있는 선택 확장입니다.
// Handler는 초기화 없이 같은 보호된 진입점을 반환해야 합니다.
type NetHTTPHandlerProvider interface {
	Handler() http.Handler
}

// NetHTTPHandler는 필수 HTTPAdapter를 net/http로 강제 변환하지 않습니다.
// native 진입점이 없으면 미지원 오류, 선언한 handler가 nil이면 구성 오류입니다.
func NetHTTPHandler(adapter HTTPAdapter) (http.Handler, error) {
	if isNil(adapter) {
		return nil, fmt.Errorf("%w: adapter is nil", ErrInvalidAdapter)
	}
	provider, ok := adapter.(NetHTTPHandlerProvider)
	if !ok {
		return nil, fmt.Errorf("%w: NetHTTPHandler", ErrUnsupportedCapability)
	}
	handler := provider.Handler()
	if isNil(handler) {
		return nil, fmt.Errorf("%w: native handler is nil", ErrInvalidAdapter)
	}
	return handler, nil
}

// NetHTTPUpgradeRoute는 기존 net/http WS 런타임의 GET 진입점입니다.
type NetHTTPUpgradeRoute struct {
	Path    string
	Handler http.Handler
}

// NetHTTPUpgradeFactory는 기본 New와 같은 설정에 net/http 업그레이드를 추가합니다.
// 업그레이드 설치 뒤 native 훅을 실행하여 기존 생성 순서를 유지합니다.
// NewWithNetHTTPUpgrades는 Config와 경로를 검사한 뒤에만 native 훅을 실행합니다.
type NetHTTPUpgradeFactory interface {
	Factory
	NewWithNetHTTPUpgrades(Config, []NetHTTPUpgradeRoute) (HTTPAdapter, error)
}

// ValidateNetHTTPUpgradeRoutes는 handler 실행 없이 경로의 누락과 nil을 검사합니다.
// 미지원 엔진 판정은 ValidateFactory의 Requirements.NetHTTPUpgrade로 수행합니다.
func ValidateNetHTTPUpgradeRoutes(routes []NetHTTPUpgradeRoute) error {
	for i, route := range routes {
		if route.Path == "" || isNil(route.Handler) {
			return fmt.Errorf("%w: Upgrades[%d] requires a path and handler", ErrInvalidAdapter, i)
		}
	}
	return nil
}
