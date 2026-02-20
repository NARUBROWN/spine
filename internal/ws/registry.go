package ws

import (
	"sync"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/router"
)

type Registration struct {
	Path string
	Meta core.HandlerMeta
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

func (r *Registry) Register(path string, handler any) {
	if path == "" {
		panic("ws: path가 빈 값일 수 없습니다")
	}
	if handler == nil {
		panic("ws: handler가 nil일 수 없습니다")
	}

	meta, err := router.NewHandlerMeta(handler)
	if err != nil {
		panic(err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.registrations = append(r.registrations, Registration{
		Path: path,
		Meta: meta,
	})
}

func (r *Registry) Registrations() []Registration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cpy := make([]Registration, len(r.registrations))
	copy(cpy, r.registrations)
	return cpy
}
