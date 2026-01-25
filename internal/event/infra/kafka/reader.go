package kafka

import (
	"context"
	"errors"

	"github.com/NARUBROWN/spine/internal/event/consumer"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/segmentio/kafka-go"
)

type Reader struct {
	reader *kafka.Reader
	opts   boot.KafkaOptions
}

func NewKafkaReader(topic string, opts boot.KafkaOptions) (*Reader, error) {
	if len(opts.Brokers) == 0 {
		return nil, errors.New("Kafka Brokers가 설정되지 않았습니다")
	}
	if opts.Read == nil {
		return nil, errors.New("Kafka Read 옵션이 설정되지 않았습니다")
	}
	if opts.Read.GroupID == "" {
		return nil, errors.New("Kafka Read GroupID가 비어 있습니다")
	}
	if topic == "" {
		return nil, errors.New("Kafka topic이 비어 있습니다")
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: opts.Brokers,
		Topic:   topic,
		GroupID: opts.Read.GroupID,
	})

	return &Reader{
		reader: reader,
		opts:   opts,
	}, nil
}

func (r *Reader) Read(ctx context.Context) (*consumer.Message, error) {
	m, err := r.reader.FetchMessage(ctx)
	if err != nil {
		return nil, err
	}

	msg := &consumer.Message{
		EventName: m.Topic,
		Payload:   m.Value,
	}

	// ACK 콜백 설정: 핸들러 성공 시 커밋
	msg.SetAckHandler(func() error {
		return r.reader.CommitMessages(context.Background(), m)
	})

	// NACK 콜백 설정: Kafka는 명시적 NACK이 없으므로 커밋하지 않음
	// (컨슈머 그룹 재시작 시 재처리됨)
	msg.SetNackHandler(func() error {
		// Kafka는 명시적 NACK 대신 커밋하지 않으면 됨
		return nil
	})

	return msg, nil
}

func (r *Reader) Close() error {
	return r.reader.Close()
}
