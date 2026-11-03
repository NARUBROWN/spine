// Package httpcontract는 Spine 커널과 플랫폼 구현이 공유하는 공개 HTTP 실행 계약입니다.
// 표준 라이브러리만 사용하며 커널 구현, Echo와 도메인 이벤트 타입을 참조하지 않습니다.
package httpcontract

import (
	"context"
	"encoding/json"
	"mime/multipart"
	"net"
	"net/http"
)

const ContractVersion = "spine.http-adapter/v2"

// HTTPContext는 HTTP 요청 하나의 입력과 실제 응답 writer를 제공합니다.
// 요청 store, 라우팅 결과와 도메인 이벤트 버스는 커널이 별도 뷰에 추가합니다.
type HTTPContext interface {
	Context() context.Context
	Method() string
	Path() string
	Param(string) string
	Params() map[string]string
	Query(string) string
	Queries() map[string][]string
	// Header와 Headers는 Host를 Request.Host에서 보충한 조회 뷰를 제공합니다.
	// Host에는 포트를 포함한 요청 값을 그대로 사용하며 전달 헤더로 대체하지 않습니다.
	Header(string) string
	// Headers의 반환값을 바꾸어도 원래 요청 헤더가 변경되면 안 됩니다.
	Headers() map[string][]string
	// JSON 바인딩은 일반 값 타입의 명시적 null을 거부합니다.
	// nil을 표현할 수 있는 타입과 사용자 정의 JSON 디코더는 각자의 규칙을 따릅니다.
	Bind(any) error
	MultipartForm() (*multipart.Form, error)
	Writer() ResponseWriter
}

// ResponseWriter는 실제 HTTP 출력 계약입니다. 응답 staging은 커널이 수행합니다.
type ResponseWriter interface {
	SetHeader(key, value string)
	AddHeader(key, value string)
	IsCommitted() bool
	WriteStatus(int) error
	// WriteJSON는 커널에서 직렬화한 JSON을 출력합니다. 사용자 DTO를 받지 않습니다.
	WriteJSON(int, json.RawMessage) error
	WriteString(int, string) error
	WriteBytes(int, []byte) error
}

// ExecuteFunc는 요청 수명 안에서 한 번 동기 호출하는 커널 실행 진입점입니다.
// 반환된 오류를 native 오류 처리기에 다시 전달해 중복 응답하지 않습니다.
type ExecuteFunc func(HTTPContext) error

// UpgradeRoute는 기존 WS 런타임에 raw HTTP 입출력을 전달할 GET 경로입니다.
type UpgradeRoute struct {
	Path    string
	Handler http.Handler
}

// Config는 리스너 없이 어댑터 인스턴스를 준비하는 설정입니다.
type Config struct {
	Options  ServerOptions
	Execute  ExecuteFunc
	Upgrades []UpgradeRoute
}

// Factory는 지원 기능을 선언하고 새 어댑터를 구성합니다.
type Factory interface {
	Capabilities() Capabilities
	New(Config) (HTTPAdapter, error)
}

// HTTPAdapter는 리스너 없는 HTTP 처리와 한 번의 관리 서버 실행을 분리합니다.
type HTTPAdapter interface {
	Handler() http.Handler
	// Serve는 nil이 아닌 리스너를 소유하고 모든 반환 경로에서 닫습니다.
	// 정상 종료는 http.ErrServerClosed이며 같은 인스턴스는 재실행하지 않습니다.
	Serve(net.Listener) error
	// Shutdown은 새 HTTP 수용을 중단하고 활성 요청을 기다립니다.
	// timeout 이후에는 새 context로 다시 기다릴 수 있습니다. WS 종료는 커널이 맡습니다.
	Shutdown(context.Context) error
}
