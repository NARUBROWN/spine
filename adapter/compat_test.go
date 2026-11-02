package adapter_test

import (
	"errors"
	"testing"

	"github.com/NARUBROWN/spine/adapter"
	httpadapter "github.com/NARUBROWN/spine/adapter/http"
)

// 기존 factory와 새 factory는 어댑터 없이 서로 대입할 수 있어야 합니다.
var (
	_ httpadapter.Factory       = adapter.Factory(nil)
	_ adapter.Factory           = httpadapter.Factory(nil)
	_ httpadapter.Config        = adapter.Config{}
	_ httpadapter.ServerOptions = adapter.ServerOptions{}
)

func TestHTTPCompatibilityPreservesTypeAndErrorIdentity(t *testing.T) {
	if adapter.ContractVersion != httpadapter.ContractVersion {
		t.Fatal("호환 경로가 HTTP 계약 버전을 바꾸면 안 됩니다")
	}
	if adapter.ErrInvalidAdapter != httpadapter.ErrInvalidAdapter ||
		adapter.ErrInvalidState != httpadapter.ErrInvalidState ||
		adapter.ErrUnsupportedCapability != httpadapter.ErrUnsupportedCapability {
		t.Fatal("두 경로의 오류 정체성이 같아야 합니다")
	}
	err := adapter.ValidateFactory(nil, adapter.Requirements{})
	if !errors.Is(err, httpadapter.ErrInvalidAdapter) {
		t.Fatalf("호환 검증이 같은 오류 종류를 반환해야 합니다: %v", err)
	}
	cause := errors.New("bind failed")
	legacy := &adapter.RequestError{Status: 400, Cause: cause}
	var canonical *httpadapter.RequestError
	if !errors.As(legacy, &canonical) || canonical != legacy || !errors.Is(canonical, cause) {
		t.Fatal("타입 변환과 원인 오류가 경로 간에 보존되어야 합니다")
	}
}
