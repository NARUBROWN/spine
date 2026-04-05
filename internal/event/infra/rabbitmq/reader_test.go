package rabbitmq

import (
	"context"
	"strings"
	"testing"

	"github.com/rabbitmq/amqp091-go"
)

type fakeAcknowledger struct {
	ackCalled    bool
	ackTag       uint64
	ackMultiple  bool
	nackCalled   bool
	nackTag      uint64
	nackMultiple bool
	nackRequeue  bool
}

func (a *fakeAcknowledger) Ack(tag uint64, multiple bool) error {
	a.ackCalled = true
	a.ackTag = tag
	a.ackMultiple = multiple
	return nil
}

func (a *fakeAcknowledger) Nack(tag uint64, multiple bool, requeue bool) error {
	a.nackCalled = true
	a.nackTag = tag
	a.nackMultiple = multiple
	a.nackRequeue = requeue
	return nil
}

func (a *fakeAcknowledger) Reject(tag uint64, requeue bool) error { return nil }

func TestNewRabbitMqReader_Validation(t *testing.T) {
	if _, err := NewRabbitMqReader(RabbitMqOptions{}); err == nil {
		t.Fatal("Read 옵션이 없으면 에러여야 합니다")
	}
	if _, err := NewRabbitMqReader(RabbitMqOptions{
		Read: &RabbitMqReadOptions{},
	}); err == nil {
		t.Fatal("Exchange가 비어 있으면 에러여야 합니다")
	}
}

func TestReader_ReadContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reader := &Reader{msgs: make(chan amqp091.Delivery)}
	_, err := reader.Read(ctx)
	if err != context.Canceled {
		t.Fatalf("context cancel 에러가 반환되어야 합니다: %v", err)
	}
}

func TestReader_ReadClosedChannel(t *testing.T) {
	msgs := make(chan amqp091.Delivery)
	close(msgs)

	reader := &Reader{msgs: msgs}
	_, err := reader.Read(context.Background())
	if err == nil || !strings.Contains(err.Error(), "채널이 닫혔습니다") {
		t.Fatalf("닫힌 채널 에러가 반환되어야 합니다: %v", err)
	}
}

func TestReader_ReadBuildsMessageAndAckNack(t *testing.T) {
	msgs := make(chan amqp091.Delivery, 1)
	ack := &fakeAcknowledger{}
	msgs <- amqp091.Delivery{
		Acknowledger: ack,
		DeliveryTag:  7,
		Type:         "order.created",
		Body:         []byte(`{"id":1}`),
		RoutingKey:   "orders.created",
	}

	reader := &Reader{msgs: msgs}
	msg, err := reader.Read(context.Background())
	if err != nil {
		t.Fatalf("Read 실패: %v", err)
	}

	if msg.EventName != "order.created" {
		t.Fatalf("event name이 잘못되었습니다: %s", msg.EventName)
	}
	if string(msg.Payload) != `{"id":1}` {
		t.Fatalf("payload가 잘못되었습니다: %s", string(msg.Payload))
	}
	if msg.Metadata["routing_key"] != "orders.created" {
		t.Fatalf("metadata가 잘못되었습니다: %+v", msg.Metadata)
	}

	if err := msg.Ack(); err != nil {
		t.Fatalf("Ack 실패: %v", err)
	}
	if !ack.ackCalled || ack.ackTag != 7 || ack.ackMultiple {
		t.Fatalf("Ack 매핑이 잘못되었습니다: %+v", ack)
	}

	if err := msg.Nack(); err != nil {
		t.Fatalf("Nack 실패: %v", err)
	}
	if !ack.nackCalled || ack.nackTag != 7 || ack.nackMultiple || !ack.nackRequeue {
		t.Fatalf("Nack 매핑이 잘못되었습니다: %+v", ack)
	}
}

func TestReaderAndWriter_CloseNilSafe(t *testing.T) {
	if err := (&Reader{}).Close(); err != nil {
		t.Fatalf("Reader.Close는 nil-safe 해야 합니다: %v", err)
	}
	if err := (&Writer{}).Close(); err != nil {
		t.Fatalf("Writer.Close는 nil-safe 해야 합니다: %v", err)
	}
}
