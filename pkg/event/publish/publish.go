package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/NARUBROWN/spine/internal/event/emission"
)

// EventPublisher는 현재 실행에서 발생한 도메인 이벤트를 받습니다.
type EventPublisher = emission.Publisher

// WithPublisher는 발행 호출을 publisher에 전달하는 자식 컨텍스트를 반환합니다.
// 핸들러를 직접 호출하는 테스트에서는 기록용 발행기를 연결할 수 있습니다.
// ctx와 publisher는 nil일 수 없습니다.
// 앱에서 브로커를 설정할 때는 spine.WithEventAdapter를 사용합니다.
func WithPublisher(ctx context.Context, publisher EventPublisher) context.Context {
	return emission.WithPublisher(ctx, publisher)
}

// DomainEvents는 타입으로 정의한 도메인 이벤트를 현재 실행에 등록합니다.
func DomainEvents(ctx context.Context, events ...DomainEvent) {
	if publisher := emission.FromContext(ctx); publisher != nil {
		publisher.Publish(events...)
	}
}

// Fields는 JSON 이벤트 본문에 담을 필드입니다.
type Fields map[string]any

// Event는 본문을 JSON으로 직렬화해 현재 실행의 발행 대기열에 넣습니다.
// 반환 오류는 등록 실패를 뜻합니다. 브로커 전송은 실행 후처리에서 처리합니다.
func Event(ctx context.Context, destination string, fields Fields) error {
	if strings.TrimSpace(destination) == "" {
		return errors.New("event destination is empty")
	}
	if ctx == nil {
		return errors.New("event context is nil")
	}
	publisher := emission.FromContext(ctx)
	if publisher == nil {
		return errors.New("event publisher is missing from context")
	}
	if fields == nil {
		fields = Fields{}
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("serialize event (%s): %w", destination, err)
	}
	publisher.Publish(fieldEvent{destination: destination, at: time.Now(), payload: payload})
	return nil
}

type fieldEvent struct {
	destination string
	at          time.Time
	payload     json.RawMessage
}

func (e fieldEvent) Name() string                 { return e.destination }
func (e fieldEvent) OccurredAt() time.Time        { return e.at }
func (e fieldEvent) MarshalJSON() ([]byte, error) { return e.payload.MarshalJSON() }
