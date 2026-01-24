package consumer

import (
	"sync"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/router"
)

type Registration struct {
	Topic string
	Meta  core.HandlerMeta
}

type Registry struct {
	mu            sync.RWMutex
	registrations []Registration
}

func NewRegistry() *Registry {
	return &Registry{
		registrations: make([]Registration, 0),
	}
}

func (r *Registry) Register(topic string, target any) {
	if topic == "" {
		panic("consumer: 토픽이 빈 값일 수 없습니다")
	}
	if target == nil {
		panic("consumer: target이 nil일 수 없습니다")
	}

	meta, err := router.NewHandlerMeta(target)

	if err != nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.registrations = append(r.registrations, Registration{
		Topic: topic,
		Meta:  meta,
	})
}

func (r *Registry) Registrations() []Registration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cpy := make([]Registration, len(r.registrations))
	copy(cpy, r.registrations)
	return cpy
}
