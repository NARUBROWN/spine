package spine

import (
	"context"
	"net/http"

	"github.com/NARUBROWN/spine/internal/bootstrap"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// ErrInvalidContext는 Prepare와 Close에 nil context를 전달했음을 나타냅니다.
var ErrInvalidContext = bootstrap.ErrInvalidContext

// PreparedApplication은 리스너 없이 준비한 앱과 획득한 자원의 소유권입니다.
type PreparedApplication interface {
	// Handler는 같은 실행 진입점을 반환합니다. HTTP가 비활성화돼 있으면 nil입니다.
	// Close 시작 뒤에는 새 요청에 503을 반환합니다. 외부 HTTP 서버는 호출자가 종료합니다.
	Handler() http.Handler
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
