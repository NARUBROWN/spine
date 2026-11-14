package main

import (
	"context"

	"github.com/NARUBROWN/spine/examples/kafka-publish/destinations"
	"github.com/NARUBROWN/spine/pkg/event/publish"
)

type OrderService struct{}

func (s *OrderService) Constructor() *OrderService { return s }

func (s *OrderService) Create(ctx context.Context, orderID int64) error {
	return publish.Event(ctx, destinations.OrderCreated, publish.Fields{
		"orderId": orderID,
	})
}
