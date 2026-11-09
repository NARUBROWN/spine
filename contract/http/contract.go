// Package httpcontract는 엔진 중립 HTTP v3 계약을 정의합니다.
// 표준 라이브러리만 사용하고 특정 플랫폼이나 다른 공개 계약을 참조하지 않습니다.
package httpcontract

import (
	"context"
	"encoding/json"
	"io"
	"net"
)

const ContractVersion = "spine.http-adapter/v3"

// HTTPContext는 요청 수명 안에서 동기 사용하는 원시 입력과 출력 뷰입니다.
// DTO 파싱, 쿼리 해석, 라우팅 결과와 요청 store는 코어가 소유합니다.
// 반환한 문자열과 헤더 스냅샷은 native 버퍼 재사용으로 바뀌면 안 됩니다.
type HTTPContext interface {
	// Context는 nil이 아니며 풀링된 native 요청 객체를 그대로 노출하지 않습니다.
	// 요청 종료 때 취소하고 native 값의 재사용으로 Value 결과가 오염되지 않게 합니다.
	// 연결 해제 취소 지원은 Capabilities.ClientDisconnectCancellation으로 구분합니다.
	Context() context.Context
	Method() string
	// RawPath는 쿼리, scheme과 authority를 제외한 디코딩 전 경로입니다.
	// native 엔진의 dot 정리나 추가 디코딩을 적용하기 전 값을 제공합니다.
	RawPath() string
	// RawQuery는 선행 물음표를 제외한 원문이며 빈 문자열도 유효합니다.
	// 파싱한 쿼리를 재직렬화한 값으로 대체하지 않습니다.
	RawQuery() string
	// Header는 이름의 대소문자를 구분하지 않으며 Host의 포트를 보존합니다.
	// Host는 실제 요청 authority를 사용하고 전달 헤더로 대체하지 않습니다.
	Header(string) string
	// Headers는 Header와 같은 값의 독립 맵이며 슬라이스 수정도 원본과 격리합니다.
	Headers() map[string][]string
	// Body는 nil이 아니며 반복 호출해도 같은 읽기 위치를 나타냅니다.
	// getter는 추가 읽기, native 폼 파싱이나 본문 재생을 시작하지 않습니다.
	// 코어는 reader를 닫지 않으며 플랫폼이 원본 스트림을 정리합니다.
	Body() io.Reader
	Writer() ResponseWriter
}

// RawQueryContext는 원시 쿼리만 필요한 코어 뷰의 투영입니다.
// v3 HTTPContext에서는 필수이며 빈 쿼리와 미제공을 bool로 구분하지 않습니다.
type RawQueryContext interface {
	RawQuery() string
}

// ResponseWriter는 코어가 확정한 응답을 플랫폼에 전달합니다.
// 메서드 반환 뒤 전달한 bytes는 재사용할 수 있어야 합니다.
// 성공 반환과 IsCommitted는 클라이언트 수신이나 실제 소켓 송신 완료를 보장하지 않습니다.
type ResponseWriter interface {
	SetHeader(key, value string)
	AddHeader(key, value string)
	IsCommitted() bool
	WriteStatus(int) error
	WriteJSON(int, json.RawMessage) error
	WriteString(int, string) error
	WriteBytes(int, []byte) error
}

// ExecuteFunc는 요청 안에서 한 번 동기 실행합니다.
// 코어가 응답을 처리하므로 반환 오류를 native 오류 처리기로 다시 응답하지 않습니다.
// 파서 정리는 완료 훅 뒤 Execute가 반환하기 전에 끝내며 원본 스트림은 닫지 않습니다.
type ExecuteFunc func(HTTPContext) error

// Config는 네트워크와 native 훅을 실행하기 전에 검사하는 구성입니다.
// native 업그레이드는 선택 factory 확장으로 전달합니다.
type Config struct {
	Options ServerOptions
	// Serve는 선택 인자를 생략한 실행의 기본값입니다.
	Serve   ServeOptions
	Execute ExecuteFunc
}

// Factory는 리스너와 실행 작업 없이 어댑터를 준비합니다.
// New는 Config.Validate를 통과한 구성만 수용해야 합니다.
type Factory interface {
	Capabilities() Capabilities
	New(Config) (HTTPAdapter, error)
}

// HTTPAdapter는 특정 엔진의 native handler 없이 실행과 종료를 제공합니다.
// 관리 서버와 외부 native 서버는 같은 요청 차단 및 추적 범위를 사용해야 합니다.
type HTTPAdapter interface {
	// Serve는 nil이 아닌 리스너를 소유하고 모든 반환 경로에서 닫습니다.
	// 인스턴스당 한 번 실행하며 정상 종료는 ErrServerClosed로 나타냅니다.
	// 예상하지 못한 nil 반환은 계약 위반입니다.
	// options가 있으면 준비한 기본값에 적용하고 실행 전에 다시 검사합니다.
	Serve(net.Listener, ...ServeOption) error
	// StopAccepting은 새 실행 진입을 동기적으로 차단하고 반복 호출할 수 있습니다.
	// 차단 뒤 native handler에 도착한 요청은 503이며 기존 요청은 보존합니다.
	// native middleware, native 경로와 업그레이드 진입도 같은 바깥 경계로 감쌉니다.
	StopAccepting()
	// Shutdown은 새 진입 차단과 관리 서버/진행 요청의 종료를 기다립니다.
	// context는 대기만 제한하고 정리는 계속되며 새 context로 다시 기다릴 수 있습니다.
	// 외부 서버 자체와 장기 WS 연결의 종료는 각각 외부 소유자와 코어가 맡습니다.
	// 종료를 시작한 인스턴스는 재실행하지 않습니다.
	Shutdown(context.Context) error
}
