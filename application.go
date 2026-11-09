package spine

import (
	"context"
	"fmt"
	"github.com/NARUBROWN/spine/contract/description"
	"net/http"
	"reflect"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/internal/bootstrap"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// ErrInvalidContext는 Prepare와 Close에 nil context를 전달했음을 나타냅니다.
var ErrInvalidContext = bootstrap.ErrInvalidContext

// PreparedApplication은 리스너 없이 준비한 앱과 획득한 자원의 소유권입니다.
type PreparedApplication interface {
	// HTTPAdapter는 준비한 동일 어댑터를 반환합니다. HTTP 비활성화 상태는 nil입니다.
	// 자원 소유권은 PreparedApplication에 있으며 실행과 종료를 중복 호출하지 않습니다.
	HTTPAdapter() httpcontract.HTTPAdapter
	// Close는 새 요청을 중단하고 진행 요청, WS와 준비한 자원을 정리합니다.
	// context는 완료 대기만 제한합니다. 시간 초과 뒤에도 정리는 계속되며 다시 기다릴 수 있습니다.
	Close(context.Context) error
}

// Prepare는 등록과 옵션을 복사한 뒤 DI, HTTP/WS와 필요한 readiness를 준비합니다.
// 리스너, OS 신호 구독, consumer와 custom transport의 Start는 실행하지 않습니다.
// 설정에 따라 broker 연결과 custom Init이 실행될 수 있습니다. ctx는 준비 중 취소에 사용하며,
// 성공한 결과는 호출자가 Close해야 합니다. 등록 변경과 Prepare를 동시에 호출하지 않습니다.
// 임의의 DI 객체에 Close를 자동 호출하는 기능은 추가하지 않습니다.
func (a *app) Prepare(ctx context.Context, options boot.Options) (PreparedApplication, error) {
	prepared, err := bootstrap.Prepare(ctx, a.bootstrapConfig(options))
	if err != nil {
		return nil, err
	}
	return prepared, nil
}

// NetHTTPHandler는 준비한 앱에서 선택적인 net/http 진입점을 조회합니다.
// 외부 서버는 호출자가 종료하며 앱 자원은 PreparedApplication.Close로 정리합니다.
func NetHTTPHandler(prepared PreparedApplication) (http.Handler, error) {
	invalid := prepared == nil
	if !invalid {
		value := reflect.ValueOf(prepared)
		switch value.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
			invalid = value.IsNil()
		}
	}
	if invalid {
		return nil, fmt.Errorf("%w: prepared application is nil", httpcontract.ErrInvalidAdapter)
	}
	adapter := prepared.HTTPAdapter()
	if adapter == nil {
		return nil, fmt.Errorf("%w: HTTP runtime is disabled", httpcontract.ErrUnsupportedCapability)
	}
	return httpcontract.NetHTTPHandler(adapter)
}

// ErrRoutesNotCollected는 생성자를 실행하지 않는 조회에서 미수집 컨트롤러를 발견했음을 나타냅니다.
var ErrRoutesNotCollected = bootstrap.ErrRoutesNotCollected

// DescribePreparedHTTP는 준비된 앱의 명세 스냅샷을 조회합니다. 생성자나 등록 함수를 다시 실행하지 않습니다.
func DescribePreparedHTTP(prepared PreparedApplication) (description.HTTP, error) {
	if prepared == nil || reflect.ValueOf(prepared).Kind() == reflect.Pointer && reflect.ValueOf(prepared).IsNil() {
		return description.HTTP{}, ErrHTTPDescriptionUnavailable
	}
	if describer, ok := prepared.(interface {
		DescribeHTTP() (description.HTTP, error)
	}); ok {
		return describer.DescribeHTTP()
	}
	return description.HTTP{}, ErrHTTPDescriptionUnavailable
}
