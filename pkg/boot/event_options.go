package boot

import (
	"math"
	"time"
)

// EventOptions는 브로커와 무관한 이벤트 실행 정책입니다.
type EventOptions struct {
	ConsumerRetry ConsumerRetryOptions
	// SettlementTimeout은 처리 결과를 확인하는 Ack/Nack의 최대 대기 시간입니다. 기본값은 5초입니다.
	SettlementTimeout time.Duration
}

// ConsumerRetryOptions는 Reader 오류 또는 실패한 ACK/NACK 뒤 전송 계층을 다시 연결하는 방식을 제어합니다.
// 별도로 지정하지 않으면 안전한 기본값인 최초 지연 100ms, 최대 지연 5초,
// 배수 2, 지터 20%, 무제한 재시도를 사용합니다.
type ConsumerRetryOptions struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	Jitter       float64
	MaxAttempts  int
}

func (o ConsumerRetryOptions) ValidateIssues(prefix string) []ConfigIssue {
	var issues []ConfigIssue
	if o.InitialDelay < 0 {
		issues = append(issues, ConfigIssue{configPath(prefix, "InitialDelay"), "CONSUMER_RETRY_INITIAL_DELAY_INVALID", "Initial delay cannot be negative.", "Use zero for the default or a positive duration."})
	}
	if o.MaxDelay < 0 {
		issues = append(issues, ConfigIssue{configPath(prefix, "MaxDelay"), "CONSUMER_RETRY_MAX_DELAY_INVALID", "Maximum delay cannot be negative.", "Use zero for the default or a positive duration."})
	}
	if o.InitialDelay > 0 && o.MaxDelay > 0 && o.MaxDelay < o.InitialDelay {
		issues = append(issues, ConfigIssue{configPath(prefix, "MaxDelay"), "CONSUMER_RETRY_DELAY_RANGE_INVALID", "Maximum delay is shorter than the initial delay.", "Set MaxDelay greater than or equal to InitialDelay."})
	}
	if math.IsNaN(o.Multiplier) || math.IsInf(o.Multiplier, 0) || o.Multiplier < 0 || (o.Multiplier > 0 && o.Multiplier < 1) {
		issues = append(issues, ConfigIssue{configPath(prefix, "Multiplier"), "CONSUMER_RETRY_MULTIPLIER_INVALID", "Multiplier must be at least 1.", "Use zero for the default or a value of 1 or greater."})
	}
	if math.IsNaN(o.Jitter) || math.IsInf(o.Jitter, 0) || o.Jitter < 0 || o.Jitter > 1 {
		issues = append(issues, ConfigIssue{configPath(prefix, "Jitter"), "CONSUMER_RETRY_JITTER_INVALID", "Jitter must be between 0 and 1.", "Use zero for the default or a fraction such as 0.2."})
	}
	if o.MaxAttempts < 0 {
		issues = append(issues, ConfigIssue{configPath(prefix, "MaxAttempts"), "CONSUMER_RETRY_MAX_ATTEMPTS_INVALID", "Maximum attempts cannot be negative.", "Use zero for unlimited retries or a positive attempt count."})
	}
	return issues
}
