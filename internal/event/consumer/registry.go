package consumer

import (
	"fmt"
	"slices"
	"sync"

	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/router"
)

type Registration struct {
	Topic string
	Meta  execution.HandlerMeta
}

type Registry struct {
	mu            sync.RWMutex
	registrations []Registration
	topics        map[string]struct{}
}

func NewRegistry() *Registry {
	return &Registry{
		registrations: make([]Registration, 0),
		topics:        make(map[string]struct{}),
	}
}

func (r *Registry) Register(topic string, target any) error {
	if topic == "" {
		return fmt.Errorf("consumer: topic cannot be empty")
	}
	if target == nil {
		return fmt.Errorf("consumer: target cannot be nil")
	}

	meta, err := router.NewHandlerMeta(target)

	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.topics == nil {
		r.topics = make(map[string]struct{})
	}
	if _, exists := r.topics[topic]; exists {
		return fmt.Errorf("consumer: topic already registered: %s", topic)
	}
	r.registrations = append(r.registrations, Registration{
		Topic: topic,
		Meta:  meta,
	})
	r.topics[topic] = struct{}{}
	return nil
}

func (r *Registry) Registrations() []Registration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cpy := make([]Registration, len(r.registrations))
	copy(cpy, r.registrations)
	return cpy
}

// Snapshot은 등록 목록과 메타데이터 slice를 복사해 준비 결과를 원본 등록 변경에서 분리합니다.
// 컨트롤러와 인터셉터 인스턴스 자체는 복제하지 않습니다.
func (r *Registry) Snapshot() *Registry {
	snapshot := NewRegistry()
	snapshot.registrations = r.Registrations()
	for i := range snapshot.registrations {
		meta := &snapshot.registrations[i].Meta
		meta.PathKeys = slices.Clone(meta.PathKeys)
		meta.Interceptors = slices.Clone(meta.Interceptors)
		snapshot.topics[snapshot.registrations[i].Topic] = struct{}{}
	}
	return snapshot
}
