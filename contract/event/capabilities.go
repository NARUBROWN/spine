package eventcontract

import (
	"fmt"
	"reflect"
)

// Capabilities는 factory의 계약 버전과 지원하는 전송 역할입니다.
// 하나 이상의 역할이 필요하며 ACK 방식이나 전달 보장 수준은 이 플래그로 추론하지 않습니다.
type Capabilities struct {
	ContractVersion string
	Consume         bool
	Publish         bool
}

// Requirements는 애플리케이션이 선택한 어댑터에 요구하는 전송 역할입니다.
type Requirements struct {
	Consume bool
	Publish bool
}

// ValidateFactory는 기능 선언만 검사하며 Reader나 Publisher를 생성하지 않습니다.
// nil, 버전 불일치와 역할 없는 선언은 ErrInvalidAdapter, 미지원 역할은 ErrUnsupportedCapability로 감쌉니다.
// 생성 결과의 nil 여부, readiness와 실제 전달 동작은 이 검사로 확인하지 않습니다.
func ValidateFactory(factory Factory, required Requirements) error {
	if isNilFactory(factory) {
		return fmt.Errorf("%w: factory is nil", ErrInvalidAdapter)
	}

	capabilities := factory.Capabilities()
	if capabilities.ContractVersion != ContractVersion {
		return fmt.Errorf("%w: contract version %q, want %q", ErrInvalidAdapter, capabilities.ContractVersion, ContractVersion)
	}
	if !capabilities.Consume && !capabilities.Publish {
		return fmt.Errorf("%w: factory must support Consume or Publish", ErrInvalidAdapter)
	}
	if required.Consume && !capabilities.Consume {
		return fmt.Errorf("%w: Consume", ErrUnsupportedCapability)
	}
	if required.Publish && !capabilities.Publish {
		return fmt.Errorf("%w: Publish", ErrUnsupportedCapability)
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
