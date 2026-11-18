package main

import (
	"context"
	"errors"
	"testing"

	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/event/publish"
)

type examplePublisher struct {
	t   *testing.T
	err error
}

func (p examplePublisher) Event(ctx context.Context, destination string, fields publish.Fields, options ...publish.Option) error {
	if ctx.Err() != nil || destination != "orders.created" || fields["orderId"] != int64(42) {
		p.t.Errorf("unexpected publish: %s %v", destination, fields)
	}
	return p.err
}

func TestOrderServiceUsesInjectedPublisher(t *testing.T) {
	want := errors.New("broker unavailable")
	for _, result := range []error{nil, want} {
		service := (&OrderService{}).Constructor(examplePublisher{t: t, err: result})
		if err := service.Create(t.Context(), 42); !errors.Is(err, result) {
			t.Fatalf("publish result=%v want=%v", err, result)
		}
	}
}

func TestKafkaExamplePreflightWithoutBroker(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "localhost:19092")
	t.Setenv("KAFKA_INSECURE", "true")
	app, err := BuildApplication()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Validate(boot.Options{HTTP: &boot.HTTPOptions{}}); err != nil {
		t.Fatal(err)
	}
}
