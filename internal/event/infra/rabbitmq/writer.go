package rabbitmq

import (
	"context"
	"encoding/json"
	"log"

	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/rabbitmq/amqp091-go"
)

type Writer struct {
	conn       *amqp091.Connection
	channel    *amqp091.Channel
	exchange   string
	routingKey string
}

func NewRabbitMqWriter(opts boot.RabbitMqOptions) *Writer {

	conn, err := amqp091.Dial(opts.URL)
	if err != nil {
		return nil
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil
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
		return nil
	}

	log.Println("[RabbitMQ][Write] 이벤트 발행기 초기화 완료")

	return &Writer{
		conn:       conn,
		channel:    ch,
		exchange:   opts.Write.Exchange,
		routingKey: opts.Write.RoutingKey,
	}
}

func (w *Writer) Publish(ctx context.Context, event publish.DomainEvent) error {

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return w.channel.PublishWithContext(
		ctx,
		w.exchange,
		w.routingKey,
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
