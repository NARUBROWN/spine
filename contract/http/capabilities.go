package httpcontract

import (
	"fmt"
	"reflect"
)

// Capabilities는 필수 HTTP 처리 외에 지원하는 기능을 선언합니다.
// ClientDisconnectCancellation은 요청 종료 시의 공통 취소와 별개입니다.
type Capabilities struct {
	ContractVersion              string
	NetHTTPHandler               bool
	NetHTTPUpgrade               bool
	ClientDisconnectCancellation bool
	ServerOptions                ServerOptionSupport
}

// Requirements는 선택한 실행 방식에 필요한 기능입니다.
// 기존 net/http WS v1을 사용하는 앱은 NetHTTPUpgrade를 요구합니다.
type Requirements struct {
	NetHTTPHandler               bool
	NetHTTPUpgrade               bool
	ClientDisconnectCancellation bool
}

// ValidateFactory는 New, native 훅과 업그레이드 생성을 실행하지 않습니다.
// 버전/선언 오류와 요청한 기능의 미지원을 구분합니다.
func ValidateFactory(factory Factory, required Requirements) error {
	if isNil(factory) {
		return fmt.Errorf("%w: factory is nil", ErrInvalidAdapter)
	}
	caps := factory.Capabilities()
	if err := validateVersion(caps); err != nil {
		return err
	}
	if caps.NetHTTPUpgrade {
		if _, ok := factory.(NetHTTPUpgradeFactory); !ok {
			return fmt.Errorf("%w: NetHTTPUpgrade requires NetHTTPUpgradeFactory", ErrInvalidAdapter)
		}
	}
	for _, feature := range []struct {
		name                string
		required, supported bool
	}{
		{"NetHTTPHandler", required.NetHTTPHandler, caps.NetHTTPHandler},
		{"NetHTTPUpgrade", required.NetHTTPUpgrade, caps.NetHTTPUpgrade},
		{"ClientDisconnectCancellation", required.ClientDisconnectCancellation, caps.ClientDisconnectCancellation},
	} {
		if feature.required && !feature.supported {
			return fmt.Errorf("%w: %s", ErrUnsupportedCapability, feature.name)
		}
	}
	return nil
}

// ValidateAdapter는 New 결과와 선언한 native handler의 유효성을 검사합니다.
// Serve, StopAccepting과 Shutdown을 호출하지 않습니다.
func ValidateAdapter(adapter HTTPAdapter, caps Capabilities) error {
	if err := validateVersion(caps); err != nil {
		return err
	}
	if isNil(adapter) {
		return fmt.Errorf("%w: factory returned a nil adapter", ErrInvalidAdapter)
	}
	if caps.NetHTTPHandler {
		if _, err := NetHTTPHandler(adapter); err != nil {
			return fmt.Errorf("%w: declared NetHTTPHandler: %w", ErrInvalidAdapter, err)
		}
	}
	return nil
}

func validateVersion(caps Capabilities) error {
	if caps.ContractVersion != ContractVersion {
		return fmt.Errorf("%w: contract version %q, want %q", ErrInvalidAdapter, caps.ContractVersion, ContractVersion)
	}
	return nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
