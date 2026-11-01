// Package adapter는 Spine 애플리케이션과 HTTP 서버 구현 사이의 실행 계약을 정의합니다.
package adapter

import (
	"context"
	"net"
	"net/http"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
)

const (
	// ContractVersion은 이 패키지가 정의하는 HTTP 어댑터 규약의 버전입니다.
	ContractVersion = "spine.http-adapter/v1"
	// ResponseWriterKey는 실제 HTTP 출력을 수행하는 core.ResponseWriter의 store 키입니다.
	ResponseWriterKey = "spine.response_writer"
	// ParamsKey는 Spine 라우터가 해석한 경로 인자를 보관하는 store 키입니다.
	ParamsKey = "spine.params"
	// PathKeysKey는 Spine 라우터가 해석한 경로 인자 이름을 보관하는 store 키입니다.
	PathKeysKey = "spine.pathKeys"
)

// HTTPContext는 HTTP 요청 하나의 실행 정보와 입력 접근을 함께 제공합니다.
// 요청이 끝난 뒤 context, writer와 업로드 파일을 재사용해서는 안 됩니다.
type HTTPContext interface {
	core.ExecutionContext
	core.HttpRequestContext
}

// ExecuteFunc는 어댑터가 내부 파이프라인 타입을 참조하지 않고 요청을 실행하는 진입점입니다.
// 어댑터는 ResponseWriterKey를 설정한 뒤 요청마다 한 번 동기 호출해야 합니다.
// 반환된 오류를 native 오류 처리기로 넘겨 응답을 다시 쓰지 않습니다.
type ExecuteFunc func(HTTPContext) error

// UpgradeRoute는 기존 WebSocket 런타임으로 연결할 GET 경로입니다.
// 어댑터는 raw HTTP writer와 request를 Handler에 전달하고 직접 업그레이드하지 않습니다.
type UpgradeRoute struct {
	Path    string
	Handler http.Handler
}

// Config는 HTTP 어댑터 인스턴스 하나를 준비하는 설정입니다.
// factory는 보관할 가변 목록을 복사해야 하며, 함수와 handler 내부 상태는 호출자가 관리합니다.
type Config struct {
	// Options의 GlobalPrefix는 bootstrap이 Spine 라우터에 한 번 적용합니다.
	// WebSocket 정책은 WS 런타임이 적용하며 어댑터가 중복 적용하지 않습니다.
	Options boot.HTTPOptions
	// Execute는 Spine HTTP fallback에 도달한 요청만 처리합니다.
	Execute ExecuteFunc
	// Upgrades는 fallback 등록 후 사용자 native 훅 전에 마운트합니다.
	Upgrades []UpgradeRoute
	// NativeHooks는 어댑터의 구체 객체를 받아 등록 순서대로 한 번 실행됩니다.
	NativeHooks []func(any)
}

// Factory는 지원 기능을 선언하고 리스너 없이 HTTP 어댑터를 구성합니다.
type Factory interface {
	// Capabilities는 초기화나 훅 호출 없이 고정된 기능 선언을 반환합니다.
	Capabilities() Capabilities
	// New는 입력을 검증한 뒤 경로와 훅을 등록하고 새 인스턴스를 반환합니다.
	// 리스너와 서버 실행을 시작하지 않으며, 실패 시 자체 부분 자원을 정리합니다.
	New(Config) (HTTPAdapter, error)
}

// HTTPAdapter는 리스너 없는 HTTP 처리와 관리되는 서버 실행을 분리합니다.
type HTTPAdapter interface {
	// Handler는 재초기화 없이 준비된 동일 HTTP 진입점을 반환합니다.
	// 외부 서버가 실행한다면 해당 서버의 리스너와 종료는 외부 소유자가 관리합니다.
	Handler() http.Handler
	// Serve는 인스턴스당 한 번 실행할 수 있고 서버 실행 동안 block합니다.
	// 전달받은 nil이 아닌 리스너를 소유하며 반환 시 닫아야 합니다.
	// 정상 종료는 http.ErrServerClosed이며 nil 반환은 계약 위반입니다.
	Serve(net.Listener) error
	// Shutdown은 HTTP 서버의 수용을 중단하고 활성 요청의 종료를 기다립니다.
	// Serve와 동시에 호출할 수 있으며 timeout 뒤 새 context로 다시 기다릴 수 있습니다.
	// WebSocket과 다른 런타임의 종료는 애플리케이션이 별도로 관리합니다.
	Shutdown(context.Context) error
}
