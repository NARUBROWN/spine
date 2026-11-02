package httpadapter

import (
	"fmt"
	"time"
)

// ServerOptions는 서버 구현에만 전달하는 설정입니다.
// 라우트 prefix, WS 정책, DI와 broker 설정은 커널이 소유합니다.
type ServerOptions struct {
	DisableRecover    bool
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
	MaxBodyBytes      int64
}

// Validate는 네트워크와 native 훅을 실행하지 않고 서버 설정을 검사합니다.
func (o ServerOptions) Validate() error {
	for _, field := range []struct {
		name  string
		value time.Duration
	}{
		{"ReadHeaderTimeout", o.ReadHeaderTimeout}, {"ReadTimeout", o.ReadTimeout},
		{"WriteTimeout", o.WriteTimeout}, {"IdleTimeout", o.IdleTimeout},
	} {
		if field.value < 0 {
			return fmt.Errorf("%w: %s cannot be negative", ErrInvalidAdapter, field.name)
		}
	}
	if o.MaxHeaderBytes < 0 {
		return fmt.Errorf("%w: MaxHeaderBytes cannot be negative", ErrInvalidAdapter)
	}
	return nil
}
