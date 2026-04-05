package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/rabbitmq/amqp091-go"
)

type Writer struct {
	conn     *amqp091.Connection
	channel  *amqp091.Channel
	exchange string
}

func NewRabbitMqWriter(opts boot.RabbitMqOptions) (*Writer, error) {
	if opts.Write == nil {
		return nil, errors.New("RabbitMQ Write 옵션이 설정되지 않았습니다")
	}

	conn, err := amqp091.Dial(opts.URL)
	if err != nil {
		return nil, fmt.Errorf("RabbitMQ 연결 실패: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("RabbitMQ 채널 생성 실패: %w", err)
	}

	err = ch.ExchangeDeclare(
		opts.Write.Exchange,
		"topic",
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,
	)

	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("RabbitMQ Exchange 선언 실패: %w", err)
	}

	log.Println("[RabbitMQ][Write] 이벤트 발행기 초기화 완료")

	return &Writer{
		conn:     conn,
		channel:  ch,
		exchange: opts.Write.Exchange,
	}, nil
}

func (w *Writer) Publish(ctx context.Context, event publish.DomainEvent) error {

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return w.channel.PublishWithContext(
		ctx,
		w.exchange,
		event.Name(),
		false,
		false,
		amqp091.Publishing{
			ContentType: "application/json",
			Body:        payload,
			Timestamp:   event.OccurredAt(),
			Type:        event.Name(),
		},
	)
}

func (w *Writer) Close() error {
	if w.channel != nil {
		_ = w.channel.Close()
	}
	if w.conn != nil {
		return w.conn.Close()
	}
	return nil
}
