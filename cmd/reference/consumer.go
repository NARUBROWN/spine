package main

import "log"

type OrderConsumer struct{}

func NewOrderConsumer() *OrderConsumer {
	return &OrderConsumer{}
}

func (c *OrderConsumer) OnCreatedKafka(eventName string, event OrderCreated) error {
	log.Println("Event received:", eventName)
	log.Println("Order ID:", event.OrderID)

	return nil
}

func (c *OrderConsumer) OnCreatedRabbitMQ(eventName string, event OrderCreated) error {
	log.Println("Event received:", eventName)
	log.Println("Order ID:", event.OrderID)

	return nil
}
