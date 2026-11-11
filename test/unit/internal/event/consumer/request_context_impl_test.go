package consumer

import (
	"context"
	"sync"
	"testing"

	"github.com/NARUBROWN/spine/internal/event/emission"
	"github.com/NARUBROWN/spine/internal/event/publish"
	"github.com/NARUBROWN/spine/internal/execution"
)

func TestConsumerContextParallelViewsShareOnePublisher(t *testing.T) {
	for _, supplied := range []publish.EventBus{nil, publish.NewEventBus()} {
		request := NewRequestContext(context.Background(), &Message{}, supplied)
		view := execution.NewContext(request).ExecutionContext()
		start := make(chan struct{})
		publishers := make(chan emission.Publisher, 32)
		var workers sync.WaitGroup
		for range cap(publishers) {
			workers.Add(1)
			go func() {
				defer workers.Done()
				<-start
				publishers <- emission.FromContext(view.Context())
			}()
		}
		close(start)
		workers.Wait()
		close(publishers)
		want := request.EventBus()
		for got := range publishers {
			if got == nil || got != want || supplied != nil && got != supplied {
				t.Fatal("병렬 뷰와 주입된 버스가 같은 publisher를 사용해야 합니다")
			}
		}
	}
}
