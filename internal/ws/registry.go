package ws

import (
	"fmt"
	"slices"
	"sync"

	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/router"
)

type Registration struct {
	Path string
	Meta execution.HandlerMeta
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

// Snapshot은 등록 목록과 메타데이터 slice를 복사해 준비 결과를 원본 등록 변경에서 분리합니다.
// 컨트롤러와 인터셉터 인스턴스 자체는 복제하지 않습니다.
func (r *Registry) Snapshot() *Registry {
	snapshot := NewRegistry()
	snapshot.registrations = r.Registrations()
	for i := range snapshot.registrations {
		meta := &snapshot.registrations[i].Meta
		meta.PathKeys = slices.Clone(meta.PathKeys)
		meta.Interceptors = slices.Clone(meta.Interceptors)
	}
	return snapshot
}
