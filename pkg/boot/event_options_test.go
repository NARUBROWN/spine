package boot

import (
	"math"
	"testing"
	"time"
)

func TestConsumerRetryOptionsValidateEveryBound(t *testing.T) {
	if issues := (ConsumerRetryOptions{}).ValidateIssues("Events.Retry"); len(issues) != 0 {
		t.Fatal(issues)
	}
	issues := (ConsumerRetryOptions{InitialDelay: 2 * time.Second, MaxDelay: time.Second, Multiplier: .5, Jitter: 1.1, MaxAttempts: -1}).ValidateIssues("Events.Retry")
	if len(issues) != 4 {
		t.Fatalf("경계 오류를 모두 반환해야 합니다: %+v", issues)
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if issues := (ConsumerRetryOptions{Multiplier: value, Jitter: value}).ValidateIssues("Retry"); len(issues) != 2 {
			t.Fatalf("유한하지 않은 재시도 설정을 허용했습니다: %+v", issues)
		}
	}
}

func TestConfigErrorFormatting(t *testing.T) {
	err := (&ConfigError{Issues: []ConfigIssue{{Path: "EventAdapters[0].Name", Code: "EVENT_ADAPTER_NAME_INVALID", Message: "Name is required.", Hint: "Assign a name."}}}).Error()
	want := "configuration validation failed with 1 issue(s):\n- EventAdapters[0].Name [EVENT_ADAPTER_NAME_INVALID]: Name is required.\n  Hint: Assign a name."
	if err != want {
		t.Fatalf("구조화 오류 형식이 다릅니다:\n%s", err)
	}
}
