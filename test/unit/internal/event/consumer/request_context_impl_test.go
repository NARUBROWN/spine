package consumer

import (
	"context"
	"github.com/NARUBROWN/spine/internal/execution"
	"testing"
)

func TestConsumerContextPreservesCallerContext(t *testing.T) {
	type key struct{}
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), key{}, "carried"))
	request := NewRequestContext(ctx, &Message{})
	view := execution.NewContext(request).ExecutionContext()
	if view.Context() != ctx || view.Context().Value(key{}) != "carried" {
		t.Fatal("context changed")
	}
	cancel()
	if view.Context().Err() != context.Canceled {
		t.Fatal("cancellation lost")
	}
}
