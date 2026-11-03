package consumer

import (
	"context"

	eventadapter "github.com/NARUBROWN/spine/contract/event"
)

// ErrReaderInvalidated는 현재 Reader 세션으로 다음 메시지를 읽어서는 안 된다는 뜻입니다.
// ACK/NACK 구현은 현재 세션을 닫고 다시 생성해야 메시지 전달 순서를 보존할 수 있을 때
// 이 오류를 반환할 수 있습니다.
var ErrReaderInvalidated = eventadapter.ErrReaderInvalidated

type Reader interface {
	Read(ctx context.Context) (*Message, error)
	Close() error
}
