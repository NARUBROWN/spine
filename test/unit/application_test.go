package spine

import (
	"context"
	"errors"
	"testing"

	adapter "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type nilPreparationContext struct{ context.Context }

func TestAppPrepareSupportsHTTPDisabledApp(t *testing.T) {
	application := New()
	prepared, err := application.Prepare(context.Background(), boot.Options{})
	if err != nil || prepared == nil || prepared.Handler() != nil {
		t.Fatalf("HTTP 없는 앱도 자원 소유권을 반환해야 합니다: %v", err)
	}
	if err := prepared.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAppPrepareRejectsNilContext(t *testing.T) {
	application := New()
	for _, ctx := range []context.Context{nil, (*nilPreparationContext)(nil)} {
		prepared, err := application.Prepare(ctx, boot.Options{})
		if !errors.Is(err, ErrInvalidContext) || prepared != nil {
			t.Fatalf("nil context를 거부하고 nil 결과를 반환해야 합니다: %v", err)
		}
	}
}

func TestAppPreparePreservesInitializationErrors(t *testing.T) {
	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		prepared, err := New().Prepare(ctx, boot.Options{})
		if !errors.Is(err, context.Canceled) || prepared != nil {
			t.Fatalf("준비 취소 오류를 보존하고 nil 결과를 반환해야 합니다: %v", err)
		}
	})
	t.Run("missing HTTP adapter", func(t *testing.T) {
		prepared, err := New().Prepare(context.Background(), boot.Options{HTTP: &boot.HTTPOptions{}})
		if !errors.Is(err, adapter.ErrInvalidAdapter) || prepared != nil {
			t.Fatalf("HTTP 설정 검증 오류를 보존하고 nil 결과를 반환해야 합니다: %v", err)
		}
	})
}
