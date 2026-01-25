package main

import "time"

type StockCreated struct {
	OrderID int64     `json:"order_id"`
	At      time.Time `json:"at"`
}

func (e StockCreated) Name() string {
	return "stock.created"
}

func (e StockCreated) OccurredAt() time.Time {
	return e.At
}
