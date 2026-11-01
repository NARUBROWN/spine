package main

import "time"

type OrderCreated struct {
	OrderID int64     `json:"order_id"`
	At      time.Time `json:"at"`
}

func (e OrderCreated) Name() string {
	return "order.created"
}

func (e OrderCreated) OccurredAt() time.Time {
	return e.At
}
