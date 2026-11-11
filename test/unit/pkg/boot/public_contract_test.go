package boot_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	spine "github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/pkg/boot"
)

func TestPublicBootSettingsExposeDataWithoutInternalValidationMethods(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[boot.HTTPOptions](), reflect.TypeFor[boot.WebSocketOptions](),
		reflect.TypeFor[boot.OpenAPIOptions](), reflect.TypeFor[boot.ConsumerRetryOptions](),
		reflect.TypeFor[boot.CleanupErrorHistoryOptions](), reflect.TypeFor[*boot.ConfigError](),
	} {
		for _, name := range []string{"Validate", "ValidateIssues", "DocumentPath", "Append"} {
			if _, exists := typ.MethodByName(name); exists {
				t.Fatalf("공개 설정 타입이 내부 제어 메서드를 노출합니다: %s.%s", typ, name)
			}
		}
	}
	var configError *boot.ConfigError
	err := spine.New().Validate(boot.Options{ShutdownTimeout: -time.Second})
	if !errors.As(err, &configError) || len(configError.Issues) != 1 || configError.Issues[0].Code != "SHUTDOWN_TIMEOUT_INVALID" {
		t.Fatalf("앱 검증의 공개 오류 조회 계약을 유지해야 합니다: %v", err)
	}
}
