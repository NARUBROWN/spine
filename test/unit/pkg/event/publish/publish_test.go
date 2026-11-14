package publish_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/NARUBROWN/spine/pkg/event/publish"
)

type recorder struct{ events []publish.DomainEvent }

func (r *recorder) Publish(events ...publish.DomainEvent) {
	r.events = append(r.events, events...)
}

func TestEventQueuesSerializedFields(t *testing.T) {
	r := &recorder{}
	ctx := publish.WithPublisher(context.Background(), r)
	fields := publish.Fields{"orderId": 42}
	if err := publish.Event(ctx, "orders.created", fields); err != nil {
		t.Fatal(err)
	}
	fields["orderId"] = 99
	if err := publish.Event(ctx, "orders.created", publish.Fields{"bad": make(chan int)}); err == nil {
		t.Fatal("잘못된 본문을 등록했습니다")
	}
	if len(r.events) != 1 || r.events[0].Name() != "orders.created" || r.events[0].OccurredAt().IsZero() {
		t.Fatalf("발행 등록 결과가 다릅니다: %#v", r.events)
	}
	payload, err := json.Marshal(r.events[0])
	if err != nil || string(payload) != `{"orderId":42}` {
		t.Fatalf("발행 본문이 다릅니다: %s, %v", payload, err)
	}
}
