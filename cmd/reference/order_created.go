package main

import "time"

type OrderCreated struct {
	OrderID int64     `json:"order_id"`
	At      time.Time `json:"at"`
}
