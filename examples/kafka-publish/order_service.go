package main

import (
	"context"

	"github.com/NARUBROWN/spine/examples/kafka-publish/destinations"
	"github.com/NARUBROWN/spine/pkg/event/publish"
)

type OrderService struct{ publisher publish.Publisher }

func (*OrderService) Constructor(publisher publish.Publisher) *OrderService {
	return &OrderService{publisher: publisher}
}

func (s *OrderService) Create(ctx context.Context, orderID int64) error {
	return s.publisher.Event(ctx, destinations.OrderCreated, publish.Fields{
		"orderId": orderID,
	})
}
