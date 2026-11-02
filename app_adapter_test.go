package spine

import (
	"errors"
	"testing"

	adapter "github.com/NARUBROWN/spine/adapter/http"
	"github.com/NARUBROWN/spine/pkg/boot"
)

func TestKernelRequiresExplicitHTTPAdapter(t *testing.T) {
	if err := New().Validate(boot.Options{}); err != nil {
		t.Fatalf("HTTP 없는 커널에는 어댑터가 필요하지 않습니다: %v", err)
	}
	for _, application := range []App{New(), New(WithHTTPAdapter(nil))} {
		if err := application.Validate(boot.Options{HTTP: &boot.HTTPOptions{}}); !errors.Is(err, adapter.ErrInvalidAdapter) {
			t.Fatalf("HTTP 구현 누락은 사전 검증에서 실패해야 합니다: %v", err)
		}
	}
}
