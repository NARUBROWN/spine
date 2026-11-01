package adapter

import "errors"

var (
	// ErrInvalidAdapter는 nil 객체, 잘못된 구성 또는 규약 버전 불일치를 나타냅니다.
	ErrInvalidAdapter = errors.New("invalid HTTP adapter")
	// ErrInvalidState는 중복 실행이나 종료 후 재실행 등 허용되지 않는 호출 상태를 나타냅니다.
	ErrInvalidState = errors.New("invalid HTTP adapter state")
	// ErrUnsupportedCapability는 선택한 어댑터가 요청한 기능을 지원하지 않음을 나타냅니다.
	ErrUnsupportedCapability = errors.New("unsupported HTTP adapter capability")
)
