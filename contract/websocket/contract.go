// Package wscontract는 Spine 커널과 WebSocket 플랫폼이 공유하는 공개 계약입니다.
// 표준 라이브러리만 사용하며 HTTP 플랫폼이나 특정 WebSocket 구현을 참조하지 않습니다.
package wscontract

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"
)

const ContractVersion = "spine.websocket-adapter/v1"

const (
	CloseNormalClosure     = 1000
	ClosePolicyViolation   = 1008
	CloseInternalServerErr = 1011
)

var ErrInvalidAdapter = errors.New("invalid WebSocket adapter")

// Capabilities는 플랫폼이 구현한 계약 버전을 선언합니다.
// 메시지 송수신, ping/pong, 읽기 제한과 close 처리는 모두 필수 기능입니다.
type Capabilities struct {
	ContractVersion string
}

// Options는 커널이 기본값을 적용한 연결 설정입니다. 모든 값은 양수여야 합니다.
// 출처 검사, 인증, 연결 수 제한과 ping 주기는 커널이 관리합니다.
type Options struct {
	HandshakeTimeout time.Duration
	ReadTimeout      time.Duration
	MaxMessageBytes  int64
}

func (o Options) Validate() error {
	if o.HandshakeTimeout <= 0 || o.ReadTimeout <= 0 || o.MaxMessageBytes <= 0 {
		return fmt.Errorf("%w: HandshakeTimeout, ReadTimeout and MaxMessageBytes must be positive", ErrInvalidAdapter)
	}
	return nil
}

// Factory는 네트워크 연결과 실행 작업을 시작하지 않고 어댑터를 준비합니다.
type Factory interface {
	Capabilities() Capabilities
	New(Options) (Adapter, error)
}

// Adapter는 인증과 출처 검사가 끝난 HTTP 요청을 WebSocket으로 업그레이드합니다.
// Upgrade는 여러 요청에서 동시에 호출할 수 있어야 합니다.
// 업그레이드 실패 시 구현이 HTTP 오류 응답을 작성하며 커널은 응답을 다시 쓰지 않습니다.
// 요청 context가 취소된 경우 업그레이드하지 않습니다.
type Adapter interface {
	Upgrade(http.ResponseWriter, *http.Request) (Connection, error)
}

// Connection은 업그레이드된 연결의 입출력을 제공합니다.
// ReadMessage는 text/binary 메시지만 반환하며 ping/pong/close는 구현이 처리합니다.
// 구현은 MaxMessageBytes를 적용하고 읽기를 시작할 때와 pong 수신 시 ReadTimeout을 갱신합니다.
// 메시지를 처리하는 핸들러 실행 시간은 다음 읽기의 대기 시간에 포함하지 않습니다.
// 커널은 읽기 루프 하나를 사용하며 WriteMessage/Ping/WriteClose 호출을 직렬화합니다.
// Close는 다른 메서드와 동시에 호출할 수 있고 진행 중인 읽기와 쓰기를 해제해야 합니다.
type Connection interface {
	ReadMessage() (messageType int, payload []byte, err error)
	// 쓰기 deadline은 절대 시각이며 시간이 없으면 제한을 두지 않습니다.
	WriteMessage(messageType int, payload []byte, deadline time.Time) error
	Ping(deadline time.Time) error
	// WriteClose는 close 프레임만 전송합니다. 자원 해제는 Close가 담당합니다.
	WriteClose(code int, reason string, deadline time.Time) error
	Close() error
}

// ValidateFactory는 New를 실행하지 않고 nil과 계약 버전을 검사합니다.
func ValidateFactory(factory Factory) error {
	if isNil(factory) {
		return fmt.Errorf("%w: factory is nil; select a WebSocket platform with spine.WithWebSocketAdapter", ErrInvalidAdapter)
	}
	if version := factory.Capabilities().ContractVersion; version != ContractVersion {
		return fmt.Errorf("%w: contract version %q, want %q", ErrInvalidAdapter, version, ContractVersion)
	}
	return nil
}

// ValidateAdapter는 New가 반환한 어댑터의 nil 여부를 검사합니다.
func ValidateAdapter(adapter Adapter) error {
	if isNil(adapter) {
		return fmt.Errorf("%w: factory returned a nil adapter", ErrInvalidAdapter)
	}
	return nil
}

// ValidateConnection은 Upgrade가 반환한 연결의 nil 여부를 검사합니다.
func ValidateConnection(connection Connection) error {
	if isNil(connection) {
		return fmt.Errorf("%w: adapter returned a nil connection", ErrInvalidAdapter)
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
