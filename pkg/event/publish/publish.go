package publish

import "context"

// EventPublisher는 현재 실행에서 발생한 도메인 이벤트를 받습니다.
type EventPublisher interface {
	Publish(events ...DomainEvent)
}

type publisherKey struct{}

// WithPublisher는 Event 호출을 publisher에 전달하는 자식 컨텍스트를 반환합니다.
// 핸들러를 직접 호출하는 테스트에서는 기록용 발행기를 연결할 수 있습니다.
// ctx와 publisher는 nil일 수 없습니다.
// 앱에서 브로커를 설정할 때는 spine.WithEventAdapter를 사용합니다.
func WithPublisher(ctx context.Context, publisher EventPublisher) context.Context {
	return context.WithValue(ctx, publisherKey{}, publisher)
}

func Event(ctx context.Context, events ...DomainEvent) {
	bus, ok := ctx.Value(publisherKey{}).(EventPublisher)
	if !ok || bus == nil {
		return
	}
	bus.Publish(events...)
}
