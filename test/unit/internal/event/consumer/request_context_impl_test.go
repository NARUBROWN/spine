package consumer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/internal/event/emission"
	"github.com/NARUBROWN/spine/internal/event/publish"
	"github.com/NARUBROWN/spine/internal/execution"
)

type publicationProbe struct{}

func (publicationProbe) Name() string          { return "publication-probe" }
func (publicationProbe) OccurredAt() time.Time { return time.Time{} }

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
		for got := range publishers {
			if got == nil {
				t.Fatal("공개 뷰에 발행 기능이 없습니다")
			}
			if _, ok := got.(emission.Bus); ok {
				t.Fatal("공개 뷰가 내부 큐 제어를 노출합니다")
			}
			got.Publish(publicationProbe{})
		}
		if events := request.EventBus().Drain(); len(events) != cap(publishers) {
			t.Fatalf("같은 요청 큐에 쌓인 이벤트 수: %d", len(events))
		}
	}
}
