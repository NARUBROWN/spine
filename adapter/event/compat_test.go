package eventadapter_test

import (
	"errors"
	"testing"

	legacy "github.com/NARUBROWN/spine/adapter/event"
	canonical "github.com/NARUBROWN/spine/contract/event"
)

var (
	_ canonical.Factory      = legacy.Factory(nil)
	_ legacy.Factory         = canonical.Factory(nil)
	_ canonical.Message      = legacy.Message{}
	_ canonical.Subscription = legacy.Subscription{}
)

func TestCompatibilityPreservesContractIdentity(t *testing.T) {
	if legacy.ContractVersion != canonical.ContractVersion ||
		legacy.ErrInvalidAdapter != canonical.ErrInvalidAdapter ||
		legacy.ErrInvalidState != canonical.ErrInvalidState ||
		legacy.ErrReaderInvalidated != canonical.ErrReaderInvalidated ||
		legacy.ErrUnsupportedCapability != canonical.ErrUnsupportedCapability {
		t.Fatal("계약 이동 전후의 타입과 오류가 같아야 합니다")
	}
	if err := legacy.ValidateFactory(nil, legacy.Requirements{}); !errors.Is(err, canonical.ErrInvalidAdapter) {
		t.Fatalf("호환 검증이 정식 오류를 반환해야 합니다: %v", err)
	}
}
