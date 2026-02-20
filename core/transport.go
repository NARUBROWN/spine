package core

import (
	"context"
	"reflect"
)

// CustomTransport는 Spine HTTP 파이프라인 외부에서 독립 실행되는 transport 계약입니다.
type CustomTransport interface {
	// Init은 DI Container 준비 이후 호출됩니다.
	Init(container Container) error
	// Start는 Init 이후 별도 goroutine에서 호출됩니다.
	Start() error
	// Stop은 Graceful Shutdown 시 호출됩니다.
	Stop(ctx context.Context) error
}

// Container는 CustomTransport에서 사용할 DI 접근용 Facade입니다.
type Container interface {
	Resolve(t reflect.Type) (any, error)
}
