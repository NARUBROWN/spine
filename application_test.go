package spine

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/NARUBROWN/spine/pkg/boot"
)

// App만 구현한 기존 래퍼가 선택적 준비 API 추가 뒤에도 호환되는지 확인합니다.
type legacyApplication struct{ App }
type optionalPreparer struct {
	App
	prepare func(context.Context, boot.Options) (PreparedApplication, error)
}

func (a optionalPreparer) Prepare(ctx context.Context, opts boot.Options) (PreparedApplication, error) {
	return a.prepare(ctx, opts)
}

type nilPreparationContext struct{ context.Context }

func TestPrepareSupportsHTTPDisabledAppWithoutChangingAppInterface(t *testing.T) {
	var application App = legacyApplication{App: New()}
	if _, err := Prepare(context.Background(), application, boot.Options{}); !errors.Is(err, ErrPreparationUnsupported) {
		t.Fatalf("기존 App 구현에는 준비 미지원 오류를 반환해야 합니다: %v", err)
	}
	prepared, err := Prepare(context.Background(), New(), boot.Options{})
	if err != nil || prepared == nil || prepared.Handler() != nil {
		t.Fatalf("HTTP 없는 앱도 자원 소유권을 반환해야 합니다: %v", err)
	}
	if err := prepared.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareRejectsInvalidInputsBeforeCallingOptionalPreparer(t *testing.T) {
	application := optionalPreparer{prepare: func(context.Context, boot.Options) (PreparedApplication, error) {
		t.Fatal("nil context로 외부 준비 구현을 호출하면 안 됩니다")
		return nil, nil
	}}
	for _, ctx := range []context.Context{nil, (*nilPreparationContext)(nil)} {
		if _, err := Prepare(ctx, application, boot.Options{}); !errors.Is(err, ErrInvalidContext) {
			t.Fatalf("nil context를 거부해야 합니다: %v", err)
		}
	}
	for _, application := range []App{nil, (*app)(nil)} {
		if _, err := Prepare(context.Background(), application, boot.Options{}); !errors.Is(err, ErrInvalidApplication) {
			t.Fatalf("nil 앱을 거부해야 합니다: %v", err)
		}
	}
}

type optionalPrepared struct{}

func (*optionalPrepared) Handler() http.Handler       { return nil }
func (*optionalPrepared) Close(context.Context) error { return nil }

func TestPrepareDelegatesOptionsAndPreservesFailures(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "value")
	want := errors.New("prepare failed")
	for _, result := range []PreparedApplication{nil, (*optionalPrepared)(nil)} {
		application := optionalPreparer{prepare: func(got context.Context, opts boot.Options) (PreparedApplication, error) {
			if got.Value(key{}) != "value" || opts.Address != "test" {
				t.Fatal("준비 context와 옵션을 보존해야 합니다")
			}
			return result, nil
		}}
		if _, err := Prepare(ctx, application, boot.Options{Address: "test"}); !errors.Is(err, ErrInvalidApplication) {
			t.Fatalf("nil 준비 결과를 거부해야 합니다: %v", err)
		}
	}
	application := optionalPreparer{prepare: func(context.Context, boot.Options) (PreparedApplication, error) { return nil, want }}
	if _, err := Prepare(ctx, application, boot.Options{}); !errors.Is(err, want) {
		t.Fatalf("외부 준비 오류를 보존해야 합니다: %v", err)
	}
}
