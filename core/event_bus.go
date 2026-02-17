package core

import "github.com/NARUBROWN/spine/pkg/event/publish"

// EventBus는 도메인 이벤트를 수집했다가 실행 후 한 번에 방출하기 위한 최소 계약입니다.
type EventBus interface {
	Publish(events ...publish.DomainEvent)
	Drain() []publish.DomainEvent
}
