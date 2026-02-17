package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/segmentio/kafka-go"
)

type KafkaPublisher struct {
	Writer *kafka.Writer
}

func NewKafkaPublisher(opts *boot.KafkaOptions) (*KafkaPublisher, error) {
	if opts == nil {
		return nil, errors.New("Kafka 옵션이 nil입니다")
	}
	if len(opts.Brokers) == 0 {
		return nil, errors.New("Kafka Brokers가 설정되지 않았습니다")
	}

	log.Println("[Kafka][Write] 이벤트 발행기 초기화 완료")

	return &KafkaPublisher{
		Writer: &kafka.Writer{
			Addr:     kafka.TCP(opts.Brokers...),
			Balancer: &kafka.LeastBytes{},
		},
	}, nil
}

func (p *KafkaPublisher) Publish(ctx context.Context, event publish.DomainEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("KafkaPublisher 직렬화 실패: %w", err)
	}

	return p.Writer.WriteMessages(ctx, kafka.Message{
		Topic: event.Name(),
		Value: payload,
		Time:  event.OccurredAt(),
	})
}

func (p *KafkaPublisher) Close() error {
	if p.Writer == nil {
		return nil
	}
	return p.Writer.Close()
}
