package spine

import (
	"context"
	"errors"
	"net/http"
	"reflect"

	"github.com/NARUBROWN/spine/internal/bootstrap"
	"github.com/NARUBROWN/spine/pkg/boot"
)

var (
	// ErrInvalidApplication은 nil 앱이나 nil 준비 결과를 나타냅니다.
	ErrInvalidApplication = errors.New("application is nil")
	// ErrPreparationUnsupported는 외부 App 구현이 선택적 준비 기능을 제공하지 않음을 나타냅니다.
	ErrPreparationUnsupported = errors.New("application does not support preparation")
	// ErrInvalidContext는 Prepare와 Close에 nil context를 전달했음을 나타냅니다.
	ErrInvalidContext = bootstrap.ErrInvalidContext
)

// PreparedApplication은 리스너 없이 준비한 앱과 획득한 자원의 소유권입니다.
type PreparedApplication interface {
	// Handler는 같은 실행 진입점을 반환합니다. HTTP가 비활성화돼 있으면 nil입니다.
	// Close 시작 뒤에는 새 요청에 503을 반환합니다. 외부 HTTP 서버는 호출자가 종료합니다.
	Handler() http.Handler
	// Close는 새 요청을 중단하고 진행 요청, WS와 준비한 자원을 정리합니다.
	// context는 완료 대기만 제한합니다. 시간 초과 뒤에도 정리는 계속되며 다시 기다릴 수 있습니다.
	Close(context.Context) error
}

// ApplicationPreparer는 기존 App 인터페이스를 확장하지 않는 선택적 준비 기능입니다.
type ApplicationPreparer interface {
	Prepare(context.Context, boot.Options) (PreparedApplication, error)
}

var _ ApplicationPreparer = (*app)(nil)

// Prepare는 등록과 옵션을 복사한 뒤 DI, HTTP/WS와 필요한 readiness를 준비합니다.
// 리스너, OS 신호 구독, consumer와 custom transport의 Start는 실행하지 않습니다.
// 설정에 따라 broker 연결과 custom Init이 실행될 수 있습니다. ctx는 준비 중 취소에 사용하며,
// 성공한 결과는 호출자가 Close해야 합니다. 등록 변경과 Prepare를 동시에 호출하지 않습니다.
// 임의의 DI 객체에 Close를 자동 호출하는 기능은 추가하지 않습니다.
func Prepare(ctx context.Context, application App, options boot.Options) (PreparedApplication, error) {
	if isNilValue(ctx) {
		return nil, ErrInvalidContext
	}
	if isNilValue(application) {
		return nil, ErrInvalidApplication
	}
	preparer, ok := application.(ApplicationPreparer)
	if !ok {
		return nil, ErrPreparationUnsupported
	}
	prepared, err := preparer.Prepare(ctx, options)
	if err != nil {
		return nil, err
	}
	if isNilValue(prepared) {
		return nil, ErrInvalidApplication
	}
	return prepared, nil
}

func (a *app) Prepare(ctx context.Context, options boot.Options) (PreparedApplication, error) {
	if a == nil {
		return nil, ErrInvalidApplication
	}
	prepared, err := bootstrap.Prepare(ctx, a.bootstrapConfig(options))
	if err != nil {
		return nil, err
	}
	return prepared, nil
}

func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
