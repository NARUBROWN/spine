package wsadapter_test

import (
	"errors"
	"testing"

	compat "github.com/NARUBROWN/spine/adapter/websocket"
	canonical "github.com/NARUBROWN/spine/contract/websocket"
)

var (
	_ canonical.Factory      = compat.Factory(nil)
	_ compat.Factory         = canonical.Factory(nil)
	_ canonical.Adapter      = compat.Adapter(nil)
	_ compat.Adapter         = canonical.Adapter(nil)
	_ canonical.Connection   = compat.Connection(nil)
	_ compat.Connection      = canonical.Connection(nil)
	_ canonical.Options      = compat.Options{}
	_ canonical.Capabilities = compat.Capabilities{}
)

func TestCompatibilityPreservesContractIdentity(t *testing.T) {
	if compat.ContractVersion != canonical.ContractVersion ||
		compat.CloseNormalClosure != canonical.CloseNormalClosure ||
		compat.ClosePolicyViolation != canonical.ClosePolicyViolation ||
		compat.CloseInternalServerErr != canonical.CloseInternalServerErr ||
		compat.ErrInvalidAdapter != canonical.ErrInvalidAdapter {
		t.Fatal("패키지를 옮겨도 계약 버전과 오류, 종료 코드는 같아야 합니다")
	}
	for name, err := range map[string]error{
		"factory":    compat.ValidateFactory(nil),
		"adapter":    compat.ValidateAdapter(nil),
		"connection": compat.ValidateConnection(nil),
	} {
		if !errors.Is(err, canonical.ErrInvalidAdapter) {
			t.Errorf("%s 검증이 정식 계약 오류를 반환해야 합니다: %v", name, err)
		}
	}
}
