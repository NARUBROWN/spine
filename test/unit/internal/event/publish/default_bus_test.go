package publish

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/internal/event/emission"
)

type concurrentBusEvent struct{ id int }

func (concurrentBusEvent) Name() string          { return "concurrent" }
func (concurrentBusEvent) OccurredAt() time.Time { return time.Time{} }

func TestDefaultEventBusConcurrentPublishAndDrainPreservesEveryEvent(t *testing.T) {
	const workers, perWorker = 32, 64
	bus := NewEventBus()
	start, done := make(chan struct{}), make(chan struct{})
	var writers sync.WaitGroup
	for worker := range workers {
		writers.Add(1)
		go func() {
			defer writers.Done()
			<-start
			for index := range perWorker {
				bus.Publish(concurrentBusEvent{id: worker*perWorker + index})
			}
		}()
	}
	go func() { writers.Wait(); close(done) }()
	close(start)
	var collected []emission.DomainEvent
drain:
	for {
		collected = append(collected, bus.Drain()...)
		select {
		case <-done:
			break drain
		default:
			runtime.Gosched()
		}
	}
	collected = append(collected, bus.Drain()...)
	seen := make(map[int]bool, workers*perWorker)
	for _, value := range collected {
		id := value.(concurrentBusEvent).id
		if seen[id] {
			t.Fatalf("이벤트가 중복 전달됐습니다: %d", id)
		}
		seen[id] = true
	}
	if len(seen) != workers*perWorker || len(bus.Drain()) != 0 {
		t.Fatalf("모든 이벤트를 한 번씩 전달해야 합니다: %d/%d", len(seen), workers*perWorker)
	}
}
