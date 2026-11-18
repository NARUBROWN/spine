package main

import (
	"log"

	"github.com/NARUBROWN/spine/pkg/spine"
)

type OrderConsumer struct{}

func NewOrderConsumer() *OrderConsumer {
	return &OrderConsumer{}
}

func (c *OrderConsumer) OnCreatedKafka(ctx spine.EventCtx, eventName string, event OrderCreated) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	log.Println("Event received:", eventName)
	log.Println("Order ID:", event.OrderID)

	return nil
}

func (c *OrderConsumer) OnCreatedRabbitMQ(ctx spine.EventCtx, eventName string, event StockCreated) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	log.Println("Event received:", eventName)
	log.Println("Stock ID:", event.StockID)

	return nil
}
