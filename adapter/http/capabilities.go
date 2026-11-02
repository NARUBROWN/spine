package httpadapter

import (
	"fmt"
	"reflect"
)

// Capabilities는 factory의 규약 버전과 선택적 지원 기능입니다.
// 기본 HTTP 처리는 필수이며 별도의 기능 플래그를 두지 않습니다.
type Capabilities struct {
	ContractVersion  string
	WebSocketUpgrade bool
}

// Requirements는 애플리케이션에 등록된 경로가 요구하는 기능입니다.
type Requirements struct {
	WebSocketUpgrade bool
}

// ValidateFactory는 어댑터를 생성하거나 훅을 실행하지 않고 버전과 지원 기능을 검사합니다.
// nil factory와 버전 불일치는 ErrInvalidAdapter, 미지원 기능은 ErrUnsupportedCapability로 감쌉니다.
// factory가 반환하는 어댑터와 handler의 유효성은 New 이후 별도로 검사해야 합니다.
func ValidateFactory(factory Factory, required Requirements) error {
	if isNilFactory(factory) {
		return fmt.Errorf("%w: factory is nil", ErrInvalidAdapter)
	}

	capabilities := factory.Capabilities()
	if capabilities.ContractVersion != ContractVersion {
		return fmt.Errorf("%w: contract version %q, want %q", ErrInvalidAdapter, capabilities.ContractVersion, ContractVersion)
	}

	if required.WebSocketUpgrade && !capabilities.WebSocketUpgrade {
		return fmt.Errorf("%w: WebSocketUpgrade", ErrUnsupportedCapability)
	}
	return nil
}

func isNilFactory(factory Factory) bool {
	if factory == nil {
		return true
	}
	value := reflect.ValueOf(factory)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
