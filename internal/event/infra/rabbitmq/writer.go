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
		return nil, errors.New("RabbitMQ write options are not configured")
	}

	conn, err := amqp091.Dial(opts.URL)
	if err != nil {
		return nil, fmt.Errorf("RabbitMQ connection failed: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("RabbitMQ channel creation failed: %w", err)
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
		return nil, fmt.Errorf("RabbitMQ exchange declaration failed: %w", err)
	}

	log.Println("[RabbitMQ][Write] Event publisher initialized")

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
