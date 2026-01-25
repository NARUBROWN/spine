package main

import (
	"context"
	"log"
)

type OrderConsumer struct{}

func NewOrderConsumer() *OrderConsumer {
	return &OrderConsumer{}
}

func (c *OrderConsumer) OnCreatedKafka(ctx context.Context, eventName string, event OrderCreated) error {
	log.Println("이벤트 수신:", eventName)
	log.Println("주문 ID:", event.OrderID)

	return nil
}

func (c *OrderConsumer) OnCreatedRabbitMQ(ctx context.Context, eventName string, event OrderCreated) error {
	log.Println("이벤트 수신:", eventName)
	log.Println("주문 ID:", event.OrderID)

	return nil
}
