package boot

import "github.com/NARUBROWN/spine/internal/configuration"

// EventOptions는 브로커와 무관한 이벤트 실행 설정입니다.
type EventOptions = configuration.EventOptions

// ConsumerRetryOptions는 Reader 오류와 ACK/NACK 실패 뒤 재연결 정책입니다.
type ConsumerRetryOptions = configuration.ConsumerRetryOptions

// CleanupErrorHistoryOptions는 자원 정리 실패 기록의 보관 한도입니다.
type CleanupErrorHistoryOptions = configuration.CleanupErrorHistoryOptions

const (
	DefaultCleanupErrorHistoryMaxEntries = configuration.DefaultCleanupErrorHistoryMaxEntries
	DefaultCleanupErrorHistoryMaxAge     = configuration.DefaultCleanupErrorHistoryMaxAge
)
