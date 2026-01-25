package rabbitmq

import (
	"context"
	"errors"

	"github.com/NARUBROWN/spine/internal/event/consumer"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/rabbitmq/amqp091-go"
)

type Reader struct {
	conn    *amqp091.Connection
	channel *amqp091.Channel
	msgs    <-chan amqp091.Delivery
}

func NewRabbitMqReader(opts boot.RabbitMqOptions) (*Reader, error) {
	if opts.Read == nil {
		return nil, errors.New("RabbitMQ Read 옵션이 설정되지 않았습니다.")
	}
	if opts.Read.Exchange == "" {
		return nil, errors.New("RabbitMQ default exchange는 Spine에서 지원하지 않습니다.")
	}

	conn, err := amqp091.Dial(opts.URL)
	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	err = ch.ExchangeDeclare(
		opts.Read.Exchange,
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
		return nil, err
	}

	_, err = ch.QueueDeclare(
		opts.Read.Queue,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,
	)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	err = ch.QueueBind(
		opts.Read.Queue,
		opts.Read.RoutingKey,
		opts.Read.Exchange,
		false,
		nil,
	)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	msgs, err := ch.Consume(
		opts.Read.Queue,
		"",
		false, // auto-ack false
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	return &Reader{
		conn:    conn,
		channel: ch,
		msgs:    msgs,
	}, nil
}

func (r *Reader) Read(ctx context.Context) (*consumer.Message, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()

	case msg, ok := <-r.msgs:
		if !ok {
			return nil, errors.New("RabbitMQ 채널이 닫혔습니다.")
		}

		eventName := msg.Type

		consumerMsg := &consumer.Message{
			EventName: eventName,
			Payload:   msg.Body,
			Metadata: map[string]string{
				"routing_key": msg.RoutingKey,
			},
		}

		// ACK 콜백 설정: 핸들러 성공 시 ACK
		consumerMsg.SetAckHandler(func() error {
			return msg.Ack(false)
		})

		// NACK 콜백 설정: 핸들러 실패 시 NACK (requeue=true)
		consumerMsg.SetNackHandler(func() error {
			return msg.Nack(false, true) // multiple=false, requeue=true
		})

		return consumerMsg, nil
	}
}

func (r *Reader) Close() error {
	if r.channel != nil {
		_ = r.channel.Close()
	}
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}
