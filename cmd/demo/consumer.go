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
	log.Println("Event received:", eventName)
	log.Println("Order ID:", event.OrderID)

	return nil
}

func (c *OrderConsumer) OnCreatedRabbitMQ(ctx context.Context, eventName string, event OrderCreated) error {
	log.Println("Event received:", eventName)
	log.Println("Order ID:", event.OrderID)

	return nil
}
