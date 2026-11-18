package main

import "time"

type StockCreated struct {
	StockID int64     `json:"stock_id"`
	At      time.Time `json:"at"`
}
