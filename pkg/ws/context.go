package ws

import "context"

// MessageContext는 프레임의 취소 신호와 실행 중 공유하는 값을 제공합니다.
// 런타임은 완료 훅이 끝나면 저장값을 비우고 프레임을 취소합니다.
type MessageContext interface {
	context.Context
	Set(key string, value any)
	Get(key string) (any, bool)
}
