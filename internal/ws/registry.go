package ws

import (
	"fmt"
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

func (r *Registry) Register(path string, handler any) error {
	if path == "" {
		return fmt.Errorf("ws: path cannot be empty")
	}
	if handler == nil {
		return fmt.Errorf("ws: handler cannot be nil")
	}

	meta, err := router.NewHandlerMeta(handler)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.registrations = append(r.registrations, Registration{
		Path: path,
		Meta: meta,
	})
	return nil
}

func (r *Registry) Registrations() []Registration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cpy := make([]Registration, len(r.registrations))
	copy(cpy, r.registrations)
	return cpy
}
