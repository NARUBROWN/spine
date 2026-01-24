package kafka

import (
	"context"
	"encoding/json"

	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/segmentio/kafka-go"
)

type KafkaPublisher struct {
	Writer *kafka.Writer
}

func NewKafkaPublisher(opts *boot.KafkaOptions) *KafkaPublisher {
	return &KafkaPublisher{
		Writer: &kafka.Writer{
			Addr:     kafka.TCP(opts.Brokers...),
			Balancer: &kafka.LeastBytes{},
		},
	}
}

func (p *KafkaPublisher) Publish(ctx context.Context, event publish.DomainEvent) error {
	payload, _ := json.Marshal(event)

	return p.Writer.WriteMessages(ctx, kafka.Message{
		Topic: event.Name(),
		Value: payload,
		Time:  event.OccurredAt(),
	})
}
