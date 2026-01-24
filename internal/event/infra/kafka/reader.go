package kafka

import (
	"context"

	"github.com/NARUBROWN/spine/internal/event/consumer"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/segmentio/kafka-go"
)

type Reader struct {
	reader *kafka.Reader
	opts   boot.KafkaOptions
}

func NewKafkaReader(topic string, opts boot.KafkaOptions) *Reader {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: opts.Brokers, // 나중에 옵션화
		Topic:   topic,
		GroupID: opts.Read.GroupID,
	})

	return &Reader{
		reader: reader,
		opts:   opts,
	}
}

func (r *Reader) Read(ctx context.Context) (consumer.Message, error) {
	m, err := r.reader.FetchMessage(ctx)
	if err != nil {
		return consumer.Message{}, err
	}

	msg := consumer.Message{
		EventName: m.Topic,
		Payload:   m.Value,
	}

	if err := r.reader.CommitMessages(ctx, m); err != nil {
		return consumer.Message{}, err
	}

	return msg, nil
}

func (r *Reader) Close() error {
	return r.reader.Close()
}
