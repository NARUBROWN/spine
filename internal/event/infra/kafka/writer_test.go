package kafka

import (
	"context"
	"testing"
	"time"

	eventpublish "github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/segmentio/kafka-go"
)

type fakeKafkaWriter struct {
	messages []kafka.Message
}

func (w *fakeKafkaWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	w.messages = append(w.messages, msgs...)
	return nil
}

func (w *fakeKafkaWriter) Close() error { return nil }

type fakeDomainEvent struct {
	name string
	at   time.Time
}

func (e fakeDomainEvent) Name() string          { return e.name }
func (e fakeDomainEvent) OccurredAt() time.Time { return e.at }

var _ eventpublish.DomainEvent = fakeDomainEvent{}

func TestKafkaPublisher_PublishUsesTopicPrefix(t *testing.T) {
	writer := &fakeKafkaWriter{}
	publisher := &KafkaPublisher{
		writer:      writer,
		topicPrefix: "dev-",
	}

	if err := publisher.Publish(context.Background(), fakeDomainEvent{
		name: "orders.created",
		at:   time.Unix(1700000000, 0),
	}); err != nil {
		t.Fatalf("Publish 실패: %v", err)
	}

	if len(writer.messages) != 1 {
		t.Fatalf("메시지는 하나만 발행되어야 합니다. 실제=%d", len(writer.messages))
	}
	if writer.messages[0].Topic != "dev-orders.created" {
		t.Fatalf("TopicPrefix가 반영되지 않았습니다: %s", writer.messages[0].Topic)
	}
}
