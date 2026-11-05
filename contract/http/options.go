package httpcontract

import (
	"fmt"
	"time"
)

// ServerOptions는 서버 설정입니다. 계약은 기본값을 적용하지 않습니다.
// 커널은 MaxBodyBytes의 0을 32 MiB로 정규화하며 native 옵션의 0은 플랫폼이 해석합니다.
// 양수 본문 한도는 길이를 모르는 입력에도 적용하고 음수는 한도를 해제합니다.
type ServerOptions struct {
	DisableRecover    bool
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
	MaxBodyBytes      int64
}

// ServerOptionSupport는 명시적으로 요청한 native 설정의 지원 여부입니다.
// 필드 이름이 같다는 이유로 서로 다른 기한/한도 의미를 지원한다고 선언하지 않습니다.
// 본문 읽기 제한은 필수이며 별도의 지원 플래그를 두지 않습니다.
type ServerOptionSupport struct {
	DisableRecover    bool
	ReadHeaderTimeout bool
	ReadTimeout       bool
	WriteTimeout      bool
	IdleTimeout       bool
	MaxHeaderBytes    bool
}

// Validate는 값의 구조만 검사하며 지원 여부와 기본값은 바꾸지 않습니다.
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

// Validate는 Execute, 옵션 값과 명시적인 native 설정의 지원을 부작용 없이 검사합니다.
func (c Config) Validate(caps Capabilities) error {
	if err := validateVersion(caps); err != nil {
		return err
	}
	if c.Execute == nil {
		return fmt.Errorf("%w: Execute is nil", ErrInvalidAdapter)
	}
	if err := c.Options.Validate(); err != nil {
		return err
	}
	o, support := c.Options, caps.ServerOptions
	for _, option := range []struct {
		name                 string
		requested, supported bool
	}{
		{"DisableRecover", o.DisableRecover, support.DisableRecover},
		{"ReadHeaderTimeout", o.ReadHeaderTimeout != 0, support.ReadHeaderTimeout},
		{"ReadTimeout", o.ReadTimeout != 0, support.ReadTimeout},
		{"WriteTimeout", o.WriteTimeout != 0, support.WriteTimeout},
		{"IdleTimeout", o.IdleTimeout != 0, support.IdleTimeout},
		{"MaxHeaderBytes", o.MaxHeaderBytes != 0, support.MaxHeaderBytes},
	} {
		if option.requested && !option.supported {
			return fmt.Errorf("%w: %s", ErrUnsupportedOption, option.name)
		}
	}
	return nil
}
